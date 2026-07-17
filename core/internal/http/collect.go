package http

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/useragent"
)

// Limits on what one event may contain.
//
// These are guards on an unauthenticated, internet-facing endpoint. The
// numbers are generous for real traffic and small enough that a flood cannot
// turn into unbounded storage or allocation.
const (
	maxCollectBody = 8 << 10 // 8KB
	maxPathLen     = 1024
	maxReferrerLen = 512
	maxUTMLen      = 255
	maxEventName   = 64
	maxProps       = 16
	maxPropKeyLen  = 64
	maxPropValLen  = 512
)

// pageviewName is the reserved event name meaning "a page was viewed" rather
// than "something custom happened".
const pageviewName = "pageview"

// collectCORS allows any origin to post events.
//
// This has to be wide open, and that is safe here. The snippet runs on every
// tracked site -- domains Zenith cannot know in advance -- so an allowlist
// would mean editing the server every time a client is added. Nothing is
// protected by refusing: site_key is public, the endpoint only ever writes,
// and no credentials or cookies are involved, so there is no cross-origin
// authority for an attacker to borrow. The worst a hostile page can do is send
// events it could equally send with curl.
//
// The snippet posts JSON with a text/plain content type, which looks wrong and
// is deliberate: text/plain is CORS-safelisted, so the POST is a simple
// request and no preflight happens. That matters because sendBeacon always
// sends with credentials mode 'include', and the spec forbids answering a
// credentialed request with the "*" below -- the preflight would be rejected
// and the event would never leave the browser. Nothing here reads the content
// type; the body is parsed as JSON regardless.
func collectCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Deliberately no Access-Control-Allow-Credentials: with Allow-Origin
		// "*" it would be ignored anyway, and cookies have no business here.
		next.ServeHTTP(w, r)
	})
}

type collectRequest struct {
	// SiteKey is public: it ships in the snippet. It authorizes writing this
	// event and nothing else.
	SiteKey string `json:"site_key"`

	// URL is the full page URL. The server derives path and UTM parameters
	// from it, so the snippet stays tiny and the parsing rules live in one
	// place rather than in every installed copy of the script.
	URL string `json:"url"`

	// Referrer is the full referring URL. Only its hostname is stored.
	Referrer string `json:"referrer"`

	// Name is "pageview", or a custom event name.
	Name string `json:"name"`

	// Props are optional custom event properties.
	Props map[string]any `json:"props"`
}

// handleCollect accepts one analytics event.
//
// It answers 204 on success with no body: the snippet has nothing to do with a
// response, and every byte saved is a byte a visitor does not wait for.
func (s *Server) handleCollect(w http.ResponseWriter, r *http.Request) {
	var req collectRequest
	if err := decodeJSON(w, r, &req, maxCollectBody); err != nil {
		writeError(w, http.StatusBadRequest, "Malformed event payload.")
		return
	}

	if req.SiteKey == "" {
		writeError(w, http.StatusBadRequest, "Missing site_key.")
		return
	}

	// An unknown key is rejected rather than silently accepted: a site owner
	// with a typo'd key should find out now, not by wondering where their
	// traffic went. site_key is public, so this reveals nothing an attacker
	// could not read from the page itself.
	site, err := s.app.SiteBySiteKey(r.Context(), req.SiteKey)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "Unknown site key.")
		return
	}
	if err != nil {
		s.log.Error("collect: site lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	event, err := s.buildEvent(r, site, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if event == nil {
		// A bot. Accepted so the client sees no error, but not recorded:
		// counting crawlers would inflate every number on the dashboard.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := s.events.Insert(r.Context(), *event); err != nil {
		s.log.Error("collect: insert", "err", err, "site_id", site.ID)
		writeError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// buildEvent turns a request into an event, or returns nil for a bot.
func (s *Server) buildEvent(r *http.Request, site storage.Site, req collectRequest) (*storage.Event, error) {
	pageURL, err := parsePageURL(req.URL)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = pageviewName
	}
	if len(name) > maxEventName {
		return nil, errors.New("Event name is too long.")
	}

	eventType := "event"
	eventName := name
	if name == pageviewName {
		eventType = pageviewName
		eventName = ""
	}

	props, err := validateProps(req.Props)
	if err != nil {
		return nil, err
	}

	// The raw address and user agent are read here, used to derive the daily
	// visitor hash and the coarse buckets below, and then dropped. Neither is
	// stored anywhere.
	ip := clientIP(r)
	rawUA := r.UserAgent()

	agent := useragent.Parse(rawUA)
	if agent.Bot {
		return nil, nil
	}

	visitorHash, err := s.visitors.Hash(site.ID, ip, rawUA)
	if err != nil {
		return nil, errors.New("Something went wrong.")
	}

	query := pageURL.Query()

	return &storage.Event{
		SiteID:      site.ID,
		TS:          time.Now().UTC(),
		Type:        eventType,
		Path:        truncate(pageURL.Path, maxPathLen),
		Referrer:    referrerHost(req.Referrer),
		UTMSource:   truncate(query.Get("utm_source"), maxUTMLen),
		UTMMedium:   truncate(query.Get("utm_medium"), maxUTMLen),
		UTMCampaign: truncate(query.Get("utm_campaign"), maxUTMLen),
		UTMTerm:     truncate(query.Get("utm_term"), maxUTMLen),
		UTMContent:  truncate(query.Get("utm_content"), maxUTMLen),
		Country:     s.geo.Country(ip),
		Device:      agent.Device,
		Browser:     agent.Browser,
		OS:          agent.OS,
		VisitorHash: visitorHash,
		EventName:   eventName,
		Props:       props,
	}, nil
}

// parsePageURL validates the page URL the event came from.
func parsePageURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("Missing url.")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("Malformed url.")
	}
	// Anything else (javascript:, data:, file:) is not a page anyone visited.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("Url must be http or https.")
	}
	if parsed.Host == "" {
		return nil, errors.New("Url must include a host.")
	}
	return parsed, nil
}

// referrerHost reduces a referring URL to its hostname.
//
// The full referrer is deliberately discarded. Referring URLs carry paths and
// query strings that routinely leak private context -- an internal ticket, a
// search phrase, a password-reset link. The hostname is the entire question
// the dashboard asks ("where did they come from?"), so it is all we keep.
func referrerHost(raw string) string {
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}

	return truncate(strings.TrimPrefix(parsed.Hostname(), "www."), maxReferrerLen)
}

// validateProps checks custom event properties.
func validateProps(props map[string]any) (map[string]any, error) {
	if len(props) == 0 {
		return nil, nil
	}
	if len(props) > maxProps {
		return nil, errors.New("Too many event properties.")
	}

	clean := make(map[string]any, len(props))
	for key, value := range props {
		if key == "" || len(key) > maxPropKeyLen {
			return nil, errors.New("Event property names must be 1 to 64 characters.")
		}

		switch v := value.(type) {
		case string:
			if len(v) > maxPropValLen {
				return nil, errors.New("Event property values are too long.")
			}
			clean[key] = v
		case float64, bool, nil:
			// JSON numbers decode as float64. These are the scalar types a
			// property can be.
			clean[key] = v
		default:
			// Nested objects and arrays would make the stats API's property
			// breakdown meaningless, so they are rejected at the door.
			return nil, errors.New("Event property values must be text, numbers, or true/false.")
		}
	}
	return clean, nil
}

// clientIP returns the address the request came from.
//
// chi's RealIP middleware has already applied X-Forwarded-For, which is why
// this reads RemoteAddr directly. The value is used to derive the visitor hash
// and look up a country, then discarded.
func clientIP(r *http.Request) string {
	// RemoteAddr is "host:port". The port must go: it changes on every
	// connection, so leaving it in would give one visitor a new hash per
	// request and turn the unique-visitor count into a pageview count.
	// SplitHostPort handles IPv6 brackets, which hand-rolled splitting does not.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.Trim(r.RemoteAddr, "[]")
	}
	return host
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
