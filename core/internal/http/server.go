// Package http wires Zenith's HTTP surface.
package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"

	"github.com/zenith/core/internal/auth"
	"github.com/zenith/core/internal/geo"
	"github.com/zenith/core/internal/scheduler"
	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/visitor"
)

// CollectRateLimit is how many events one IP may send per minute.
//
// Generous on purpose. A single visitor sends a handful of events per page, but
// an office or a school shares one address, and rate-limiting a whole building
// into silence would corrupt the data far worse than the flood it prevents.
const CollectRateLimit = 600

// Server holds the dependencies every handler draws on.
type Server struct {
	events   storage.EventStore
	app      storage.AppStore
	issuer   *auth.Issuer
	visitors *visitor.Hasher
	geo      geo.Resolver
	log      *slog.Logger

	// reporter sends monthly reports. Nil on a deployment with no scheduler,
	// where the report endpoints answer 503 rather than panicking.
	reporter *scheduler.Reporter

	dashboardDir string
}

// Deps are the collaborators a Server needs.
type Deps struct {
	Events   storage.EventStore
	App      storage.AppStore
	Issuer   *auth.Issuer
	Visitors *visitor.Hasher
	Geo      geo.Resolver
	Log      *slog.Logger

	// Reporter sends monthly reports. Optional.
	Reporter *scheduler.Reporter

	// DashboardDir holds the built SPA. Empty means the console is not served
	// -- the API still works, which is all a headless deployment needs.
	DashboardDir string
}

// New builds a Server.
func New(d Deps) *Server {
	if d.Geo == nil {
		d.Geo = geo.Unavailable{}
	}
	return &Server{
		events:       d.Events,
		app:          d.App,
		issuer:       d.Issuer,
		visitors:     d.Visitors,
		geo:          d.Geo,
		log:          d.Log,
		reporter:     d.Reporter,
		dashboardDir: d.DashboardDir,
	}
}

// Routes returns the core HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/health", s.handleHealth)

	// The tracking snippet, for sites that install with a <script src> rather
	// than the npm package. Public and cacheable.
	r.Get("/track.js", s.handleTrackerScript)

	// Ingestion. Open to the internet by necessity: it is called by a script
	// on someone else's page, so it can carry no secret worth protecting and
	// must defend itself with limits instead.
	r.Group(func(r chi.Router) {
		r.Use(collectCORS)
		r.Use(httprate.LimitByIP(CollectRateLimit, time.Minute))

		r.Post("/api/collect", s.handleCollect)
		r.Options("/api/collect", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	})

	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/login", s.handleLogin)

		// Logout must know which token to revoke, so it authenticates first.
		r.Group(func(r chi.Router) {
			r.Use(s.RequireAuth)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleMe)
		})
	})

	// Managing sites is the developer's alone. An owner has exactly one site
	// and no business enumerating the others.
	r.Route("/api/sites", func(r chi.Router) {
		r.Use(s.RequireAuth)
		r.Use(s.RequireDeveloper)

		r.Get("/", s.handleListSites)
		r.Post("/", s.handleCreateSite)
		r.Patch("/{id}", s.handleUpdateSite)
		r.Delete("/{id}", s.handleDeleteSite)
		r.Get("/{id}/reports", s.handleSiteReports)
		r.Post("/{id}/reports/send", s.handleSendReport)
	})

	// SEO audits. Running one is the developer's to trigger; reading the
	// report is scoped to whoever may read the site it is about.
	//
	// Reads take either credential, like stats do. The client's own dashboard
	// has no session -- it acts for whoever passed the password gate on the
	// owner's site and proves itself with the site's api key -- and without
	// this its SEO tab could never load the audit the developer ran. Running
	// an audit stays behind RequireDeveloper, which an api key's owner claims
	// can never satisfy: a client cannot spend the deployment's crawl budget.
	r.Route("/api/audits", func(r chi.Router) {
		r.Use(s.RequireStatsAccess)

		r.Get("/", s.handleListAudits)
		r.Get("/{id}", s.handleAuditDetail)

		r.Group(func(r chi.Router) {
			r.Use(s.RequireDeveloper)
			r.Post("/", s.handleCreateAudit)
		})
	})

	// Global settings: the Resend key and MAIL FROM every site's report goes
	// through. Developer only, and the key never comes back out.
	r.Route("/api/settings", func(r chi.Router) {
		r.Use(s.RequireAuth)
		r.Use(s.RequireDeveloper)

		r.Get("/", s.handleGetSettings)
		r.Put("/", s.handleUpdateSettings)
	})

	// Reading analytics takes either a session token or a site's api key. Each
	// handler then resolves which site the caller may read: a developer names
	// one, an owner's is fixed by their token, an api key's by the key.
	r.Route("/api/stats", func(r chi.Router) {
		r.Use(s.RequireStatsAccess)

		r.Get("/summary", s.handleSummary)
		r.Get("/timeseries", s.handleTimeseries)
		r.Get("/pages", s.handlePages)
		r.Get("/referrers", s.handleReferrers)
		r.Get("/geo", s.handleGeo)
		r.Get("/tech", s.handleTech)
		r.Get("/events", s.handleEvents)
		r.Get("/realtime", s.handleRealtime)
	})

	// The built SPA: the console at /dashboard, and the same bundle the
	// domain-native proxy points an owner's browser at.
	if assets, ok := dashboardAssets(s.dashboardDir); ok {
		// Bare host lands on the console. Temporary, not permanent: a future
		// build may put a landing page here, and a cached 301 would be a trap.
		r.Get("/", http.RedirectHandler("/dashboard/", http.StatusFound).ServeHTTP)
		r.Handle("/dashboard", http.RedirectHandler("/dashboard/", http.StatusMovedPermanently))
		r.Handle("/dashboard/*", http.StripPrefix("/dashboard/", assets))
	}

	return r
}

type meResponse struct {
	Role      storage.Role `json:"role"`
	SiteID    string       `json:"site_id,omitempty"`
	ExpiresAt time.Time    `json:"expires_at"`
}

// handleMe reports the current session, so the SPA can tell on load whether a
// stored token is still good without guessing.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in first.")
		return
	}

	writeJSON(w, http.StatusOK, meResponse{
		Role:      claims.Role,
		SiteID:    claims.SiteID,
		ExpiresAt: claims.Expiry(),
	})
}

type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth reports liveness. It checks both stores so an unreachable
// database surfaces here rather than as a failure on the first real request.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.events.Ping(ctx); err != nil {
		s.log.Error("health: event store unreachable", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "degraded"})
		return
	}
	if err := s.app.Ping(ctx); err != nil {
		s.log.Error("health: app store unreachable", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "degraded"})
		return
	}

	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
