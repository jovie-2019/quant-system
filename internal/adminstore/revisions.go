package adminstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ParamRevision is one row in strategy_param_revisions. It captures the
// full audit trail for a single control command: what was proposed, who
// proposed it, when it was sent, and whether the runner accepted it.
type ParamRevision struct {
	ID            int64
	StrategyID    string
	Revision      int64
	CommandType   string // "update_params" | "pause" | "resume" | "shadow_on" | "shadow_off"
	ParamsBefore  string // JSON snapshot; empty for non-update commands
	ParamsAfter   string // JSON snapshot of the proposed params
	Actor         string
	Reason        string
	IssuedMS      int64
	AckReceivedMS sql.NullInt64
	AckAccepted   sql.NullBool
	AckError      sql.NullString
}

// CreateParamRevision records a new proposed command. Returns the auto-
// generated primary key. Duplicate (strategy_id, revision) tuples fail
// with a unique-constraint error — callers should advance the revision
// counter atomically before writing.
func (s *Store) CreateParamRevision(ctx context.Context, r ParamRevision) (int64, error) {
	if r.IssuedMS == 0 {
		r.IssuedMS = time.Now().UnixMilli()
	}
	const q = `
INSERT INTO strategy_param_revisions (
  strategy_id, revision, command_type, params_before, params_after, actor, reason, issued_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := s.db.ExecContext(ctx, q,
		r.StrategyID, r.Revision, r.CommandType, r.ParamsBefore, r.ParamsAfter,
		r.Actor, r.Reason, r.IssuedMS,
	)
	if err != nil {
		return 0, fmt.Errorf("adminstore: create param revision: %w", err)
	}
	return res.LastInsertId()
}

// UpdateParamRevisionAck writes the runner's ack into the audit row. The
// ack subscriber in admin-api calls this once per control command.
func (s *Store) UpdateParamRevisionAck(ctx context.Context, strategyID string, revision int64, accepted bool, errMsg string, receivedMS int64) error {
	const q = `
UPDATE strategy_param_revisions
SET ack_received_ms = ?, ack_accepted = ?, ack_error = ?
WHERE strategy_id = ? AND revision = ?
`
	acc := int64(0)
	if accepted {
		acc = 1
	}
	_, err := s.db.ExecContext(ctx, q, receivedMS, acc, errMsg, strategyID, revision)
	if err != nil {
		return fmt.Errorf("adminstore: update param revision ack: %w", err)
	}
	return nil
}

// NextRevision returns max(revision)+1 for the given strategy. Returns 1
// when no revisions exist yet. Implementations should call this under a
// transaction with the subsequent insert if strict monotonicity matters;
// for the MVP the unique-constraint on (strategy_id, revision) catches
// races with a clean error.
func (s *Store) NextRevision(ctx context.Context, strategyID string) (int64, error) {
	const q = `SELECT COALESCE(MAX(revision), 0) FROM strategy_param_revisions WHERE strategy_id = ?`
	var maxRev int64
	if err := s.db.QueryRowContext(ctx, q, strategyID).Scan(&maxRev); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 1, nil
		}
		return 0, fmt.Errorf("adminstore: next revision: %w", err)
	}
	return maxRev + 1, nil
}

// ListParamRevisions returns up to `limit` revisions for the given
// strategy, newest first. A limit of 0 defaults to 50.
func (s *Store) ListParamRevisions(ctx context.Context, strategyID string, limit int) ([]ParamRevision, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
SELECT id, strategy_id, revision, command_type, params_before, params_after, actor, reason,
       issued_ms, ack_received_ms, ack_accepted, ack_error
FROM strategy_param_revisions
WHERE strategy_id = ?
ORDER BY revision DESC
LIMIT ?
`
	rows, err := s.db.QueryContext(ctx, q, strategyID, limit)
	if err != nil {
		return nil, fmt.Errorf("adminstore: list param revisions: %w", err)
	}
	defer rows.Close()

	out := make([]ParamRevision, 0, limit)
	for rows.Next() {
		var (
			r       ParamRevision
			before  sql.NullString
			after   sql.NullString
			reason  sql.NullString
			ackErr  sql.NullString
			ackRec  sql.NullInt64
			ackOk   sql.NullBool
		)
		if err := rows.Scan(
			&r.ID, &r.StrategyID, &r.Revision, &r.CommandType,
			&before, &after, &r.Actor, &reason,
			&r.IssuedMS, &ackRec, &ackOk, &ackErr,
		); err != nil {
			return nil, fmt.Errorf("adminstore: scan param revision: %w", err)
		}
		r.ParamsBefore = before.String
		r.ParamsAfter = after.String
		r.Reason = reason.String
		r.AckReceivedMS = ackRec
		r.AckAccepted = ackOk
		r.AckError = ackErr
		out = append(out, r)
	}
	return out, rows.Err()
}
