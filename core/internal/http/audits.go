package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/zenith/core/internal/id"
	"github.com/zenith/core/internal/storage"
)

const maxAuditBody = 1 << 10

type auditJobJSON struct {
	ID     string `json:"id"`
	SiteID string `json:"site_id"`
	Status string `json:"status"`

	RequestedAt string `json:"requested_at"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`

	Score int    `json:"score"`
	Error string `json:"error,omitempty"`
}

func toAuditJobJSON(j storage.AuditJob) auditJobJSON {
	out := auditJobJSON{
		ID: j.ID, SiteID: j.SiteID, Status: j.Status,
		RequestedAt: j.RequestedAt.Format(time.RFC3339),
		Score:       j.Score, Error: j.Err,
	}
	if !j.StartedAt.IsZero() {
		out.StartedAt = j.StartedAt.Format(time.RFC3339)
	}
	if !j.FinishedAt.IsZero() {
		out.FinishedAt = j.FinishedAt.Format(time.RFC3339)
	}
	return out
}

type createAuditRequest struct {
	SiteID string `json:"site_id"`
}

// handleCreateAudit enqueues an audit and returns immediately.
//
// It does not wait: rendering every page of a site with a real browser takes
// minutes, and an HTTP request that blocks for minutes is a request that times
// out somewhere in between. The worker picks the job up; the console polls.
func (s *Server) handleCreateAudit(w http.ResponseWriter, r *http.Request) {
	var req createAuditRequest
	if err := decodeJSON(w, r, &req, maxAuditBody); err != nil {
		writeError(w, http.StatusBadRequest, "Send a site_id as JSON.")
		return
	}

	if req.SiteID == "" {
		writeError(w, http.StatusBadRequest, "Name a site to audit.")
		return
	}

	if _, err := s.app.SiteByID(r.Context(), req.SiteID); errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such site.")
		return
	} else if err != nil {
		s.log.Error("audits: site lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	// One audit at a time per site. A second is not more information -- it is
	// the same crawl again, competing for the same worker, against the same
	// site. Hand back the one already running instead.
	if existing, err := s.app.ActiveAuditForSite(r.Context(), req.SiteID); err == nil {
		writeJSON(w, http.StatusOK, toAuditJobJSON(existing))
		return
	} else if !errors.Is(err, storage.ErrNotFound) {
		s.log.Error("audits: active lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	jobID, err := id.New()
	if err != nil {
		s.log.Error("audits: generate id", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}

	job := storage.AuditJob{
		ID: jobID, SiteID: req.SiteID,
		Status: storage.AuditQueued, RequestedAt: time.Now().UTC(),
	}

	if err := s.app.CreateAuditJob(r.Context(), job); err != nil {
		s.log.Error("audits: create", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't start that audit. Try again.")
		return
	}

	s.log.Info("audit queued", "job_id", job.ID, "site_id", job.SiteID)
	writeJSON(w, http.StatusAccepted, toAuditJobJSON(job))
}

type auditsResponse struct {
	Audits []auditJobJSON `json:"audits"`

	// WorkerHint is set when audits are queued but nothing is consuming them.
	// Without it, a developer who never started the worker sees a job sit at
	// "queued" forever with no explanation.
	WorkerHint string `json:"worker_hint,omitempty"`
}

// handleListAudits returns a site's audits, newest first.
func (s *Server) handleListAudits(w http.ResponseWriter, r *http.Request) {
	site, ok := s.resolveSite(w, r)
	if !ok {
		return
	}

	jobs, err := s.app.AuditJobsForSite(r.Context(), site.ID)
	if err != nil {
		s.log.Error("audits: list", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't load audits. Try again.")
		return
	}

	out := auditsResponse{Audits: make([]auditJobJSON, 0, len(jobs))}
	for _, job := range jobs {
		out.Audits = append(out.Audits, toAuditJobJSON(job))
	}

	if hint := s.staleQueueHint(jobs); hint != "" {
		out.WorkerHint = hint
	}

	writeJSON(w, http.StatusOK, out)
}

// staleQueueHint explains a job that has been queued too long.
//
// The audit worker is optional -- a developer who does not want SEO simply
// never starts it -- so "queued forever" is a normal misconfiguration, and the
// console has to say so rather than spin.
func (s *Server) staleQueueHint(jobs []storage.AuditJob) string {
	const patience = 2 * time.Minute

	for _, job := range jobs {
		if job.Status != storage.AuditQueued {
			continue
		}
		if time.Since(job.RequestedAt) > patience {
			return "This audit has been waiting a while. Is the audit-worker service running?"
		}
	}
	return ""
}

type auditDetailResponse struct {
	Audit auditJobJSON    `json:"audit"`
	Pages []auditPageJSON `json:"pages"`
}

type auditPageJSON struct {
	URL    string          `json:"url"`
	Score  int             `json:"score"`
	Checks json.RawMessage `json:"checks"`
}

// handleAuditDetail returns one audit and its per-page findings.
func (s *Server) handleAuditDetail(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	job, err := s.app.AuditJobByID(r.Context(), jobID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such audit.")
		return
	}
	if err != nil {
		s.log.Error("audits: detail", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't load that audit. Try again.")
		return
	}

	// The audit names a site; the caller must be allowed to read that site.
	// Without this an owner could read another client's audit by guessing an
	// id -- the job id is the only thing between them and it.
	if !s.maySeeSite(r, job.SiteID) {
		writeError(w, http.StatusForbidden, "You don't have access to that.")
		return
	}

	results, err := s.app.AuditResultsForJob(r.Context(), jobID)
	if err != nil {
		s.log.Error("audits: results", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't load that audit. Try again.")
		return
	}

	out := auditDetailResponse{
		Audit: toAuditJobJSON(job),
		Pages: make([]auditPageJSON, 0, len(results)),
	}
	for _, result := range results {
		out.Pages = append(out.Pages, auditPageJSON{
			URL:    result.PageURL,
			Score:  result.Score,
			Checks: json.RawMessage(result.Checks),
		})
	}

	writeJSON(w, http.StatusOK, out)
}

// maySeeSite reports whether the caller may read a site's data.
func (s *Server) maySeeSite(r *http.Request, siteID string) bool {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		return false
	}

	switch claims.Role {
	case storage.RoleDeveloper:
		return true
	case storage.RoleOwner:
		return claims.SiteID == siteID
	default:
		return false
	}
}
