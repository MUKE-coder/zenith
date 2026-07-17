package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zenith/core/internal/auth"
	"github.com/zenith/core/internal/storage"
)

// maxLoginBody caps the login payload. An email and a password are a few
// hundred bytes; anything larger is a mistake or an attempt to make the server
// allocate on an unauthenticated path.
const maxLoginBody = 4 << 10 // 4KB

type ctxKey int

const claimsKey ctxKey = iota

// ClaimsFrom returns the authenticated caller's claims.
//
// The bool is false on any unauthenticated request, so a handler that forgets
// RequireAuth fails closed rather than reading a zero-valued identity.
func ClaimsFrom(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(*auth.Claims)
	return claims, ok
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
	Role      storage.Role `json:"role"`
	Email     string       `json:"email"`
}

// handleLogin verifies an email and password and issues a session token.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req, maxLoginBody); err != nil {
		writeError(w, http.StatusBadRequest, "Send an email and password as JSON.")
		return
	}

	email := strings.TrimSpace(req.Email)
	if email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Enter your email and password.")
		return
	}

	user, err := s.app.UserByEmail(r.Context(), email)
	if errors.Is(err, storage.ErrNotFound) {
		// Spend the same time a real check would, so response timing cannot be
		// used to discover which emails have accounts.
		_ = auth.VerifyDecoy()
		writeError(w, http.StatusUnauthorized, "Incorrect email or password.")
		return
	}
	if err != nil {
		s.log.Error("login: lookup failed", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	if err := auth.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		// Never log the password or say which half was wrong.
		s.log.Info("login: bad password", "email", user.Email)
		writeError(w, http.StatusUnauthorized, "Incorrect email or password.")
		return
	}

	if user.Role != storage.RoleDeveloper {
		// Owners authenticate through their own site's password gate, not here.
		writeError(w, http.StatusUnauthorized, "Incorrect email or password.")
		return
	}

	token, expiresAt, err := s.issuer.IssueDeveloper(user.ID)
	if err != nil {
		s.log.Error("login: issue token", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	s.log.Info("login", "user_id", user.ID, "role", user.Role)
	writeJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		Role:      user.Role,
		Email:     user.Email,
	})
}

// handleLogout revokes the token that authenticated the request.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in first.")
		return
	}

	// Revoke until the token would have expired anyway; after that the
	// signature check rejects it and the row is only clutter.
	userID := ""
	if claims.Role == storage.RoleDeveloper {
		userID = claims.Subject
	}

	if err := s.app.RevokeToken(r.Context(), claims.TokenID(), userID, claims.Expiry()); err != nil {
		s.log.Error("logout: revoke", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't sign you out. Try again.")
		return
	}

	s.log.Info("logout", "sub", claims.Subject)
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed out"})
}

// RequireAuth rejects any request without a valid, unrevoked token.
func (s *Server) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "Sign in first.")
			return
		}

		claims, err := s.issuer.Parse(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Your session has expired. Sign in again.")
			return
		}

		// A valid signature is not enough: the token may have been signed out.
		revoked, err := s.app.IsTokenRevoked(r.Context(), claims.TokenID())
		if err != nil {
			// Fail closed. If we cannot tell whether a token was revoked, we
			// must not assume it wasn't.
			s.log.Error("auth: revocation check failed", "err", err)
			writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
			return
		}
		if revoked {
			writeError(w, http.StatusUnauthorized, "Your session has expired. Sign in again.")
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// APIKeyHeader carries a site's secret api key.
//
// A header, not a query parameter: query strings land in access logs, browser
// history, and Referer headers, and this key reads a client's analytics.
const APIKeyHeader = "X-Zenith-API-Key"

// RequireStatsAccess authenticates a stats request by either credential.
//
// Two callers read analytics and they prove themselves differently. A person
// in the console sends a session token. The domain-native proxy has no session
// -- it acts for whoever passed the password gate on the owner's own site --
// so it sends the site's secret api key instead.
//
// An api key resolves to owner claims for exactly that site, so everything
// downstream treats it identically to an owner's token and cannot read wider
// than one site.
func (s *Server) RequireStatsAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey := strings.TrimSpace(r.Header.Get(APIKeyHeader)); apiKey != "" {
			site, err := s.app.SiteByAPIKey(r.Context(), apiKey)
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusUnauthorized, "Unknown API key.")
				return
			}
			if err != nil {
				s.log.Error("auth: api key lookup", "err", err)
				writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
				return
			}

			claims := &auth.Claims{Role: storage.RoleOwner, SiteID: site.ID}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		s.RequireAuth(next).ServeHTTP(w, r)
	})
}

// RequireDeveloper rejects anyone who is not a developer. It must be used
// behind RequireAuth.
func (s *Server) RequireDeveloper(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFrom(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "Sign in first.")
			return
		}
		if claims.Role != storage.RoleDeveloper {
			writeError(w, http.StatusForbidden, "You don't have access to that.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts a token from the Authorization header.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}

	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	return token, true
}

// decodeJSON reads a JSON body, capped at limit bytes and rejecting unknown
// fields so a typo'd field name is an error rather than a silent default.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Exactly one JSON value, not a stream.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON object")
	}
	return nil
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
