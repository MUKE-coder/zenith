package http

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/zenith/core/internal/email"
	"github.com/zenith/core/internal/id"
	"github.com/zenith/core/internal/storage"
)

const (
	maxSiteBody   = 4 << 10 // 4KB
	maxSiteName   = 100
	maxSiteDomain = 253 // the longest a hostname may legally be
)

type siteJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`

	// SiteKey is public: it ships in the snippet.
	SiteKey string `json:"site_key"`

	// APIKey is secret. These endpoints are developer-only, and the developer
	// owns the key -- they need it for zenith.config.js. It must never reach
	// an owner-scoped caller, which is why nothing outside /api/sites returns
	// it.
	APIKey string `json:"api_key"`

	OwnerEmail string    `json:"owner_email,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func toSiteJSON(s storage.Site) siteJSON {
	return siteJSON{
		ID: s.ID, Name: s.Name, Domain: s.Domain,
		SiteKey: s.SiteKey, APIKey: s.APIKey,
		OwnerEmail: s.OwnerEmail, CreatedAt: s.CreatedAt,
	}
}

type sitesResponse struct {
	Sites []siteJSON `json:"sites"`
}

// handleListSites returns every site. Developer scope.
func (s *Server) handleListSites(w http.ResponseWriter, r *http.Request) {
	sites, err := s.app.ListSites(r.Context())
	if err != nil {
		s.log.Error("sites: list", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't load your sites. Try again.")
		return
	}

	out := sitesResponse{Sites: make([]siteJSON, 0, len(sites))}
	for _, site := range sites {
		out.Sites = append(out.Sites, toSiteJSON(site))
	}

	writeJSON(w, http.StatusOK, out)
}

type createSiteRequest struct {
	Name       string `json:"name"`
	Domain     string `json:"domain"`
	OwnerEmail string `json:"owner_email"`
}

// handleCreateSite adds a site and generates its keys.
func (s *Server) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	var req createSiteRequest
	if err := decodeJSON(w, r, &req, maxSiteBody); err != nil {
		writeError(w, http.StatusBadRequest, "Send a name and domain as JSON.")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > maxSiteName {
		writeError(w, http.StatusBadRequest, "Give the site a name, up to 100 characters.")
		return
	}

	domain, err := normalizeDomain(req.Domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ownerEmail := strings.TrimSpace(req.OwnerEmail)
	if ownerEmail != "" && !strings.Contains(ownerEmail, "@") {
		writeError(w, http.StatusBadRequest, "That doesn't look like an email address.")
		return
	}

	siteID, err := id.New()
	if err != nil {
		s.log.Error("sites: generate id", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	// Two keys, generated together and never interchangeable: one public for
	// the snippet, one secret for reading.
	siteKey, err := id.NewSiteKey()
	if err != nil {
		s.log.Error("sites: generate site key", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}
	apiKey, err := id.NewSiteKey()
	if err != nil {
		s.log.Error("sites: generate api key", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	site := storage.Site{
		ID: siteID, Name: name, Domain: domain,
		SiteKey: siteKey, APIKey: apiKey, OwnerEmail: ownerEmail,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.app.CreateSite(r.Context(), site); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			// Two 256-bit keys colliding is not a thing that happens; this is
			// here so the impossible case cannot be mistaken for a 500.
			writeError(w, http.StatusConflict, "That site already exists.")
			return
		}
		s.log.Error("sites: create", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't add that site. Try again.")
		return
	}

	s.log.Info("site created", "site_id", site.ID, "domain", site.Domain)
	writeJSON(w, http.StatusCreated, toSiteJSON(site))
}

type updateSiteRequest struct {
	// All optional: a PATCH that only sets the owner email should not require
	// re-sending the name and domain.
	Name       *string `json:"name"`
	Domain     *string `json:"domain"`
	OwnerEmail *string `json:"owner_email"`
}

// handleUpdateSite edits a site's name, domain, and owner email.
//
// The keys are not editable here. Rotating one invalidates every installed
// snippet or every dashboard session, which is a deliberate operation and not
// something an edit form should be able to do by accident.
func (s *Server) handleUpdateSite(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "id")

	site, err := s.app.SiteByID(r.Context(), siteID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such site.")
		return
	}
	if err != nil {
		s.log.Error("sites: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	var req updateSiteRequest
	if err := decodeJSON(w, r, &req, maxSiteBody); err != nil {
		writeError(w, http.StatusBadRequest, "Send the fields to change as JSON.")
		return
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > maxSiteName {
			writeError(w, http.StatusBadRequest, "Give the site a name, up to 100 characters.")
			return
		}
		site.Name = name
	}

	if req.Domain != nil {
		domain, err := normalizeDomain(*req.Domain)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		site.Domain = domain
	}

	if req.OwnerEmail != nil {
		owner := strings.TrimSpace(*req.OwnerEmail)
		// Empty is meaningful: it turns the monthly report off for this site.
		if owner != "" && !strings.Contains(owner, "@") {
			writeError(w, http.StatusBadRequest, "That doesn't look like an email address.")
			return
		}
		site.OwnerEmail = owner
	}

	if err := s.app.UpdateSite(r.Context(), site); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "No such site.")
			return
		}
		s.log.Error("sites: update", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't save that site. Try again.")
		return
	}

	s.log.Info("site updated", "site_id", site.ID)
	writeJSON(w, http.StatusOK, toSiteJSON(site))
}

// handleDeleteSite removes a site and all of its data.
//
// Events are deleted first, then the site row. If the event delete fails, the
// site is still listed and can be retried; the reverse ordering would leave a
// site's events stranded under an id nothing references, invisible but still
// counted in nothing -- pure waste that no UI could ever surface to clean up.
func (s *Server) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "id")

	if _, err := s.app.SiteByID(r.Context(), siteID); errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such site.")
		return
	} else if err != nil {
		s.log.Error("sites: lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	if err := s.events.DeleteSite(r.Context(), siteID); err != nil {
		s.log.Error("sites: delete events", "err", err, "site_id", siteID)
		writeError(w, http.StatusInternalServerError, "Couldn't delete that site's data. Try again.")
		return
	}

	if err := s.app.DeleteSite(r.Context(), siteID); err != nil {
		s.log.Error("sites: delete", "err", err, "site_id", siteID)
		writeError(w, http.StatusInternalServerError, "Couldn't delete that site. Try again.")
		return
	}

	s.log.Info("site deleted", "site_id", siteID)
	w.WriteHeader(http.StatusNoContent)
}

type testReportResponse struct {
	Status string `json:"status"`
	SentTo string `json:"sent_to"`
}

// handleTestReport sends a site's report right now.
//
// It does not record the send: see Reporter.SendTest. A preview that marked
// the month as done would replace the report it was previewing.
func (s *Server) handleTestReport(w http.ResponseWriter, r *http.Request) {
	if s.reporter == nil {
		writeError(w, http.StatusServiceUnavailable, "Reports are not available on this deployment.")
		return
	}

	siteID := chi.URLParam(r, "id")

	site, err := s.app.SiteByID(r.Context(), siteID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such site.")
		return
	}
	if err != nil {
		s.log.Error("report: site lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	if err := s.reporter.SendTest(r.Context(), siteID); err != nil {
		if errors.Is(err, email.ErrNotConfigured) {
			writeError(w, http.StatusBadRequest,
				"Email isn't configured. Add a Resend API key and MAIL FROM in settings.")
			return
		}
		// Resend's own message is the useful part ("domain not verified"), so
		// it is passed through rather than replaced with something vaguer.
		s.log.Error("report: test send", "err", err, "site_id", siteID)
		writeError(w, http.StatusBadGateway, "Couldn't send: "+err.Error())
		return
	}

	s.log.Info("test report sent", "site_id", siteID)
	writeJSON(w, http.StatusOK, testReportResponse{Status: "sent", SentTo: site.OwnerEmail})
}

type reportsResponse struct {
	Reports []reportJSON `json:"reports"`
}

type reportJSON struct {
	Period string `json:"period"`
	Status string `json:"status"`
	SentAt string `json:"sent_at,omitempty"`
	Error  string `json:"error,omitempty"`
}

// handleSiteReports returns a site's report history, so a failure surfaces
// rather than being something the developer hears about from their client.
func (s *Server) handleSiteReports(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "id")

	reports, err := s.app.ReportsForSite(r.Context(), siteID)
	if err != nil {
		s.log.Error("reports: list", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't load report history. Try again.")
		return
	}

	out := reportsResponse{Reports: make([]reportJSON, 0, len(reports))}
	for _, report := range reports {
		row := reportJSON{Period: report.Period, Status: report.Status, Error: report.Err}
		if !report.SentAt.IsZero() {
			row.SentAt = report.SentAt.Format(time.RFC3339)
		}
		out.Reports = append(out.Reports, row)
	}

	writeJSON(w, http.StatusOK, out)
}

// normalizeDomain reduces what someone pasted to a bare hostname.
//
// People paste "https://example.com/", "www.example.com", and "Example.COM"
// interchangeably. Storing them as typed would break the self-referral filter,
// which compares a stored referrer host against this value.
func normalizeDomain(raw string) (string, error) {
	domain := strings.TrimSpace(strings.ToLower(raw))
	if domain == "" {
		return "", errors.New("Give the site a domain, like example.com.")
	}

	// Tolerate a pasted URL.
	if strings.Contains(domain, "://") {
		parsed, err := url.Parse(domain)
		if err != nil || parsed.Host == "" {
			return "", errors.New("That doesn't look like a domain.")
		}
		domain = parsed.Host
	}

	domain = strings.TrimSuffix(domain, "/")
	domain = strings.TrimPrefix(domain, "www.")

	// Drop a port, and anything after a path separator.
	if host, _, found := strings.Cut(domain, "/"); found {
		domain = host
	}
	if host, _, found := strings.Cut(domain, ":"); found {
		domain = host
	}

	if domain == "" || len(domain) > maxSiteDomain {
		return "", errors.New("That doesn't look like a domain.")
	}
	// A bare hostname has no spaces and at least one dot. Not a full RFC
	// check: the goal is to catch a typo now, not to litigate hostname syntax.
	if strings.ContainsAny(domain, " \t") || !strings.Contains(domain, ".") {
		return "", errors.New("That doesn't look like a domain. Try example.com.")
	}

	return domain, nil
}
