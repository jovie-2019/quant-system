package adminstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CandidateStatus enumerates the lifecycle of a proposed parameter set.
type CandidateStatus string

const (
	CandidatePending  CandidateStatus = "pending"
	CandidateApproved CandidateStatus = "approved"
	CandidateRejected CandidateStatus = "rejected"
	CandidateApplied  CandidateStatus = "applied"
	CandidateExpired  CandidateStatus = "expired"
)

// ParamCandidate is one row of strategy_param_candidates. Created by
// the scheduled ReoptimizeJob (Origin="scheduler_reoptimize") or by a
// manually-triggered Optuna run (Origin="manual_optimization:opt_xxx").
type ParamCandidate struct {
	ID              int64
	StrategyID      string
	Origin          string
	ProposedParams  string
	BaselineParams  string
	BaselineSharpe  sql.NullFloat64
	ProposedSharpe  sql.NullFloat64
	Improvement     sql.NullFloat64
	Status          CandidateStatus
	RejectionReason sql.NullString
	CreatedMS       int64
	ReviewedMS      sql.NullInt64
	Reviewer        sql.NullString
}

// CreateParamCandidate inserts a new pending candidate. CreatedMS
// defaults to now when left zero.
func (s *Store) CreateParamCandidate(ctx context.Context, c ParamCandidate) (int64, error) {
	if c.CreatedMS == 0 {
		c.CreatedMS = time.Now().UnixMilli()
	}
	if c.Status == "" {
		c.Status = CandidatePending
	}
	const q = `
INSERT INTO strategy_param_candidates
  (strategy_id, origin, proposed_params, baseline_params,
   baseline_sharpe, proposed_sharpe, improvement, status, created_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := s.db.ExecContext(ctx, q,
		c.StrategyID, c.Origin, c.ProposedParams, c.BaselineParams,
		c.BaselineSharpe, c.ProposedSharpe, c.Improvement,
		string(c.Status), c.CreatedMS,
	)
	if err != nil {
		return 0, fmt.Errorf("adminstore: create param candidate: %w", err)
	}
	return res.LastInsertId()
}

// GetParamCandidate fetches a row by id.
func (s *Store) GetParamCandidate(ctx context.Context, id int64) (ParamCandidate, bool, error) {
	const q = `
SELECT id, strategy_id, origin, proposed_params, baseline_params,
       baseline_sharpe, proposed_sharpe, improvement, status,
       rejection_reason, created_ms, reviewed_ms, reviewer
FROM strategy_param_candidates WHERE id = ?
`
	var c ParamCandidate
	err := s.db.QueryRowContext(ctx, q, id).Scan(
		&c.ID, &c.StrategyID, &c.Origin, &c.ProposedParams, &c.BaselineParams,
		&c.BaselineSharpe, &c.ProposedSharpe, &c.Improvement, &c.Status,
		&c.RejectionReason, &c.CreatedMS, &c.ReviewedMS, &c.Reviewer,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ParamCandidate{}, false, nil
		}
		return ParamCandidate{}, false, fmt.Errorf("adminstore: get candidate: %w", err)
	}
	return c, true, nil
}

// ListParamCandidatesFilter narrows a list query. Zero values mean "no filter".
type ListParamCandidatesFilter struct {
	StrategyID string
	Status     CandidateStatus
	Limit      int
}

// ListParamCandidates returns newest-first candidates matching the filter.
func (s *Store) ListParamCandidates(ctx context.Context, f ListParamCandidatesFilter) ([]ParamCandidate, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	q := `
SELECT id, strategy_id, origin, proposed_params, baseline_params,
       baseline_sharpe, proposed_sharpe, improvement, status,
       rejection_reason, created_ms, reviewed_ms, reviewer
FROM strategy_param_candidates
`
	var (
		args   []any
		wheres []string
	)
	if f.StrategyID != "" {
		wheres = append(wheres, "strategy_id = ?")
		args = append(args, f.StrategyID)
	}
	if f.Status != "" {
		wheres = append(wheres, "status = ?")
		args = append(args, string(f.Status))
	}
	if len(wheres) > 0 {
		q += "WHERE " + joinAnd(wheres) + "\n"
	}
	q += "ORDER BY created_ms DESC LIMIT ?"
	args = append(args, f.Limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("adminstore: list candidates: %w", err)
	}
	defer rows.Close()

	out := make([]ParamCandidate, 0, f.Limit)
	for rows.Next() {
		var c ParamCandidate
		if err := rows.Scan(
			&c.ID, &c.StrategyID, &c.Origin, &c.ProposedParams, &c.BaselineParams,
			&c.BaselineSharpe, &c.ProposedSharpe, &c.Improvement, &c.Status,
			&c.RejectionReason, &c.CreatedMS, &c.ReviewedMS, &c.Reviewer,
		); err != nil {
			return nil, fmt.Errorf("adminstore: scan candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateParamCandidateStatus transitions a candidate. For approve /
// reject, reviewer and reviewedMS are captured; for applied, reviewer
// is the actor that fired the hot-reload.
func (s *Store) UpdateParamCandidateStatus(ctx context.Context, id int64, status CandidateStatus, reviewer, rejectionReason string) error {
	const q = `
UPDATE strategy_param_candidates
SET status = ?, reviewer = ?, rejection_reason = ?, reviewed_ms = ?
WHERE id = ?
`
	_, err := s.db.ExecContext(ctx, q, string(status), reviewer, rejectionReason,
		time.Now().UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("adminstore: update candidate status: %w", err)
	}
	return nil
}

// joinAnd joins clauses with ' AND ' — trivial helper kept private to
// this file so the candidate query builder does not pull in fmt/strings
// indirections for one call.
func joinAnd(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " AND "
		}
		out += p
	}
	return out
}
