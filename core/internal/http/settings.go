package http

import (
	"net/http"
	"strings"
)

const maxSettingsBody = 4 << 10

// maskedSecret is what a configured API key looks like from outside.
//
// A fixed placeholder, not a prefix of the real key: showing the first few
// characters helps nobody identify it and helps an attacker confirm it.
const maskedSecret = "••••••••"

type settingsResponse struct {
	// ResendConfigured says whether a key is set. The key itself is never
	// returned -- not masked, not truncated, not at all. A secret that leaves
	// the server is a secret that can leak from a browser, a proxy log, or a
	// screenshot.
	ResendConfigured bool   `json:"resend_configured"`
	ResendAPIKey     string `json:"resend_api_key,omitempty"`
	MailFrom         string `json:"mail_from"`

	// EmailReady is what the console actually needs to know: whether monthly
	// reports can go out at all.
	EmailReady bool `json:"email_ready"`
}

// handleGetSettings returns the global settings, with the API key masked.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.app.Settings(r.Context())
	if err != nil {
		s.log.Error("settings: read", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't load settings. Try again.")
		return
	}

	out := settingsResponse{
		ResendConfigured: settings.ResendAPIKey != "",
		MailFrom:         settings.MailFrom,
		EmailReady:       settings.Configured(),
	}
	if out.ResendConfigured {
		out.ResendAPIKey = maskedSecret
	}

	writeJSON(w, http.StatusOK, out)
}

type updateSettingsRequest struct {
	// ResendAPIKey is optional on update. Omitted, or sent back as the mask
	// the UI was given, leaves the stored key alone -- which is what lets
	// someone change MAIL FROM without re-typing a secret they cannot see.
	ResendAPIKey *string `json:"resend_api_key"`
	MailFrom     *string `json:"mail_from"`
}

// handleUpdateSettings replaces the global settings.
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := decodeJSON(w, r, &req, maxSettingsBody); err != nil {
		writeError(w, http.StatusBadRequest, "Send settings as JSON.")
		return
	}

	current, err := s.app.Settings(r.Context())
	if err != nil {
		s.log.Error("settings: read", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't load settings. Try again.")
		return
	}

	next := current

	if req.ResendAPIKey != nil {
		key := strings.TrimSpace(*req.ResendAPIKey)
		switch key {
		case maskedSecret:
			// The UI echoed back the mask: keep what is stored.
		default:
			next.ResendAPIKey = key
		}
	}

	if req.MailFrom != nil {
		from := strings.TrimSpace(*req.MailFrom)
		if from != "" && !looksLikeMailFrom(from) {
			writeError(w, http.StatusBadRequest,
				"MAIL FROM must be an email address, like reports@example.com "+
					"or Zenith <reports@example.com>.")
			return
		}
		next.MailFrom = from
	}

	if err := s.app.UpdateSettings(r.Context(), next); err != nil {
		s.log.Error("settings: update", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't save settings. Try again.")
		return
	}

	// Never log the key, and never echo it back.
	s.log.Info("settings updated", "resend_configured", next.ResendAPIKey != "", "mail_from", next.MailFrom)

	writeJSON(w, http.StatusOK, settingsResponse{
		ResendConfigured: next.ResendAPIKey != "",
		ResendAPIKey:     maskIf(next.ResendAPIKey),
		MailFrom:         next.MailFrom,
		EmailReady:       next.Configured(),
	})
}

func maskIf(key string) string {
	if key == "" {
		return ""
	}
	return maskedSecret
}

// looksLikeMailFrom accepts "a@b.c" and "Name <a@b.c>".
//
// Not a full RFC 5322 parse: the goal is to catch a typo before Resend
// rejects it a month later, when the report was due.
func looksLikeMailFrom(v string) bool {
	addr := v

	if open := strings.LastIndex(v, "<"); open != -1 {
		if !strings.HasSuffix(v, ">") {
			return false
		}
		addr = v[open+1 : len(v)-1]
	}

	at := strings.Index(addr, "@")
	if at <= 0 || at == len(addr)-1 {
		return false
	}
	return strings.Contains(addr[at+1:], ".") && !strings.ContainsAny(addr, " \t")
}
