package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"quant-system/internal/adminstore"
	"quant-system/internal/bus/natsbus"
	"quant-system/pkg/contracts"
)

// Param-candidate REST handlers — the operator's view of the
// self-optimisation pipeline (Phase 7). The ReoptimizeJob proposes new
// params into strategy_param_candidates; this surface lets a human
// list pending rows, approve (which fires the existing hot-reload
// path) or reject (with a note), and trigger an out-of-schedule run.

// CandidateRow is the JSON projection of adminstore.ParamCandidate.
type CandidateRow struct {
	ID              int64   `json:"id"`
	StrategyID      string  `json:"strategy_id"`
	Origin          string  `json:"origin"`
	ProposedParams  string  `json:"proposed_params"`
	BaselineParams  string  `json:"baseline_params"`
	BaselineSharpe  float64 `json:"baseline_sharpe"`
	ProposedSharpe  float64 `json:"proposed_sharpe"`
	Improvement     float64 `json:"improvement"`
	Status          string  `json:"status"`
	RejectionReason string  `json:"rejection_reason,omitempty"`
	CreatedMS       int64   `json:"created_ms"`
	ReviewedMS      int64   `json:"reviewed_ms,omitempty"`
	Reviewer        string  `json:"reviewer,omitempty"`
}

// CandidateList is the list response.
type CandidateList struct {
	Items []CandidateRow `json:"items"`
	Count int            `json:"count"`
}

// HandleListParamCandidates implements GET /api/v1/param-candidates.
// Query params: ?status=pending|approved|rejected|applied&strategy_id=&limit=
func (s *Server) HandleListParamCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	q := r.URL.Query()
	f := adminstore.ListParamCandidatesFilter{
		StrategyID: q.Get("strategy_id"),
		Status:     adminstore.CandidateStatus(q.Get("status")),
	}
	if raw := q.Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 500 {
			f.Limit = v
		}
	}
	rows, err := s.store.ListParamCandidates(r.Context(), f)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	out := make([]CandidateRow, 0, len(rows))
	for _, c := range rows {
		out = append(out, toCandidateRow(c))
	}
	s.writeJSON(w, http.StatusOK, CandidateList{Items: out, Count: len(out)})
}

// HandleGetParamCandidate implements GET /api/v1/param-candidates/:id.
func (s *Server) HandleGetParamCandidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	id, err := parseCandidateID(r.URL.Path, "")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	c, ok, err := s.store.GetParamCandidate(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "candidate not found")
		return
	}
	s.writeJSON(w, http.StatusOK, toCandidateRow(c))
}

// ApproveCandidateRequest is the optional body of the approve endpoint.
type ApproveCandidateRequest struct {
	Reason string `json:"reason,omitempty"`
	Actor  string `json:"actor,omitempty"`
}

// HandleApproveParamCandidate implements POST /api/v1/param-candidates/:id/approve.
// A successful approval:
//   1. flips the candidate row's status to "approved"
//   2. fires a strategy.control update_params command to the runner
//   3. records the approval into the existing param-revisions audit
//   4. flips the row to "applied" once the NATS publish succeeds
// If any downstream step fails, the candidate stays "approved" so the
// operator can retry the dispatch with POST .../apply.
func (s *Server) HandleApproveParamCandidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	id, err := parseCandidateID(r.URL.Path, "/approve")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	cand, ok, err := s.store.GetParamCandidate(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "candidate not found")
		return
	}
	if cand.Status != adminstore.CandidatePending && cand.Status != adminstore.CandidateApproved {
		s.writeError(w, http.StatusConflict, "invalid_state",
			fmt.Sprintf("candidate is %s; only pending/approved can be approved/applied", cand.Status))
		return
	}

	var req ApproveCandidateRequest
	_ = s.readJSON(r, &req) // body is optional
	if req.Actor == "" {
		if uid, ok := userIDFromContext(r.Context()); ok {
			req.Actor = uid
		} else {
			req.Actor = "anonymous"
		}
	}
	if req.Reason == "" {
		req.Reason = fmt.Sprintf("approved candidate id=%d (origin=%s, improvement=%.3f)",
			cand.ID, cand.Origin, cand.Improvement.Float64)
	}

	// Step 1: mark approved.
	if err := s.store.UpdateParamCandidateStatus(r.Context(), id, adminstore.CandidateApproved, req.Actor, ""); err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Step 2: require the NATS bus — without it we cannot fire the update.
	if s.bus == nil {
		s.writeError(w, http.StatusServiceUnavailable, "bus_unavailable",
			"candidate marked approved but NATS is not configured; /apply will retry")
		return
	}

	// Step 3+4: reuse the same revision + NATS publish path used by the
	// manual POST /strategies/:id/params endpoint so a cancelled ack,
	// stale revision, or malformed payload is handled identically.
	rev, err := s.store.NextRevision(r.Context(), cand.StrategyID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	now := time.Now().UTC().UnixMilli()
	if _, err := s.store.CreateParamRevision(r.Context(), adminstore.ParamRevision{
		StrategyID:   cand.StrategyID,
		Revision:     rev,
		CommandType:  string(contracts.StrategyControlUpdateParams),
		ParamsBefore: cand.BaselineParams,
		ParamsAfter:  cand.ProposedParams,
		Actor:        req.Actor,
		Reason:       req.Reason,
		IssuedMS:     now,
	}); err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	env := contracts.StrategyControlEnvelope{
		StrategyID: cand.StrategyID,
		Type:       contracts.StrategyControlUpdateParams,
		Revision:   rev,
		Params:     json.RawMessage(cand.ProposedParams),
		Reason:     req.Reason,
		Actor:      req.Actor,
		IssuedMS:   now,
	}
	if err := natsbus.PublishStrategyControl(r.Context(), s.bus, env, nil); err != nil {
		s.writeError(w, http.StatusInternalServerError, "nats_publish_failed", err.Error())
		return
	}

	// Mirror the new params into strategy_configs so a runner restart
	// loads them.
	cfg, found, _ := s.store.GetStrategyConfigByStrategyID(r.Context(), cand.StrategyID)
	if found {
		cfg.ConfigJSON = cand.ProposedParams
		if err := s.store.UpdateStrategyConfig(r.Context(), cfg); err != nil {
			s.logger.Warn("candidate approve: update config_json failed",
				"error", err, "strategy_id", cand.StrategyID)
		}
	}

	// Final status: applied. The ack listener will separately close the
	// param-revision row once the runner confirms the swap.
	if err := s.store.UpdateParamCandidateStatus(r.Context(), id, adminstore.CandidateApplied, req.Actor, ""); err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]any{
		"candidate_id": id,
		"strategy_id":  cand.StrategyID,
		"revision":     rev,
		"status":       "applied",
	})
}

// RejectCandidateRequest is the body of the reject endpoint.
type RejectCandidateRequest struct {
	Reason string `json:"reason"`
	Actor  string `json:"actor,omitempty"`
}

// HandleRejectParamCandidate implements POST /api/v1/param-candidates/:id/reject.
func (s *Server) HandleRejectParamCandidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	id, err := parseCandidateID(r.URL.Path, "/reject")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	var req RejectCandidateRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Reason == "" {
		s.writeError(w, http.StatusBadRequest, "bad_request", "reason is required")
		return
	}
	cand, ok, err := s.store.GetParamCandidate(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "candidate not found")
		return
	}
	if cand.Status != adminstore.CandidatePending {
		s.writeError(w, http.StatusConflict, "invalid_state",
			fmt.Sprintf("candidate is %s; only pending can be rejected", cand.Status))
		return
	}
	if req.Actor == "" {
		if uid, ok := userIDFromContext(r.Context()); ok {
			req.Actor = uid
		}
	}
	if err := s.store.UpdateParamCandidateStatus(r.Context(), id,
		adminstore.CandidateRejected, req.Actor, req.Reason); err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"candidate_id": id,
		"status":       "rejected",
	})
}

// HandleRunReoptimizeNow implements POST /api/v1/reoptimize/run-now. The
// scheduled nightly run is configured at admin-api startup; this
// endpoint lets operators trigger an ad-hoc invocation to test a
// configuration change without waiting 24 hours.
func (s *Server) HandleRunReoptimizeNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if s.reoptimize == nil {
		s.writeError(w, http.StatusServiceUnavailable, "scheduler_disabled",
			"reoptimize job not wired; start admin-api with CLICKHOUSE_ADDR + NATS_URL")
		return
	}
	// Run synchronously so the operator gets immediate feedback. The
	// batch loop logs per-strategy progress; for long runs a client
	// can poll /api/v1/param-candidates?status=pending.
	started := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := s.reoptimize.Run(ctx); err != nil {
		s.writeError(w, http.StatusInternalServerError, "reoptimize_failed", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":     "done",
		"elapsed_ms": time.Since(started).Milliseconds(),
	})
}

func toCandidateRow(c adminstore.ParamCandidate) CandidateRow {
	row := CandidateRow{
		ID:             c.ID,
		StrategyID:     c.StrategyID,
		Origin:         c.Origin,
		ProposedParams: c.ProposedParams,
		BaselineParams: c.BaselineParams,
		Status:         string(c.Status),
		CreatedMS:      c.CreatedMS,
	}
	if c.BaselineSharpe.Valid {
		row.BaselineSharpe = c.BaselineSharpe.Float64
	}
	if c.ProposedSharpe.Valid {
		row.ProposedSharpe = c.ProposedSharpe.Float64
	}
	if c.Improvement.Valid {
		row.Improvement = c.Improvement.Float64
	}
	if c.RejectionReason.Valid {
		row.RejectionReason = c.RejectionReason.String
	}
	if c.ReviewedMS.Valid {
		row.ReviewedMS = c.ReviewedMS.Int64
	}
	if c.Reviewer.Valid {
		row.Reviewer = c.Reviewer.String
	}
	return row
}

// parseCandidateID extracts the numeric id from
// /api/v1/param-candidates/:id[/suffix]. When suffix is empty the path
// must end with just the id; when suffix is non-empty (e.g. "/approve")
// the id segment sits immediately before it.
func parseCandidateID(path, suffix string) (int64, error) {
	rest := strings.TrimPrefix(path, "/api/v1/param-candidates/")
	if rest == path {
		return 0, errors.New("bad path")
	}
	if suffix != "" {
		rest = strings.TrimSuffix(rest, suffix)
	}
	if rest == "" {
		return 0, errors.New("missing id")
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", rest)
	}
	return id, nil
}
