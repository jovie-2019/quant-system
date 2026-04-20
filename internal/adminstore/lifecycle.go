package adminstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// LifecycleTransition is one row of strategy_lifecycle_transitions. It
// captures an audit trail of every promotion / demotion / deprecate,
// so the UI can render a Gantt-style timeline even across process
// restarts.
type LifecycleTransition struct {
	ID             int64
	StrategyID     string
	FromStage      string
	ToStage        string
	Kind           string // "promote" | "demote" | "deprecate"
	Actor          string
	Reason         string
	TransitionedMS int64
}

// GetLifecycleStage returns the persisted stage for the given
// strategy_configs.id. An empty string (== default "draft") counts as
// draft so the caller can always assume a non-empty stage downstream.
func (s *Store) GetLifecycleStage(ctx context.Context, id int64) (string, error) {
	const q = `SELECT COALESCE(lifecycle_stage, 'draft') FROM strategy_configs WHERE id = ?`
	var stage string
	if err := s.db.QueryRowContext(ctx, q, id).Scan(&stage); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("adminstore: get lifecycle stage: %w", err)
	}
	if stage == "" {
		return "draft", nil
	}
	return stage, nil
}

// SetLifecycleStage atomically updates strategy_configs.lifecycle_stage
// and appends a row to strategy_lifecycle_transitions so the change is
// auditable. Callers validate the transition via the lifecycle package
// BEFORE invoking this; the store is a dumb persister.
func (s *Store) SetLifecycleStage(ctx context.Context, t LifecycleTransition) error {
	if t.TransitionedMS == 0 {
		t.TransitionedMS = time.Now().UnixMilli()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("adminstore: begin lifecycle tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE strategy_configs SET lifecycle_stage = ?, updated_ms = ? WHERE strategy_id = ?`,
		t.ToStage, t.TransitionedMS, t.StrategyID,
	); err != nil {
		return fmt.Errorf("adminstore: update lifecycle stage: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO strategy_lifecycle_transitions
		  (strategy_id, from_stage, to_stage, kind, actor, reason, transitioned_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.StrategyID, t.FromStage, t.ToStage, t.Kind, t.Actor, t.Reason, t.TransitionedMS,
	); err != nil {
		return fmt.Errorf("adminstore: insert lifecycle transition: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("adminstore: commit lifecycle tx: %w", err)
	}
	return nil
}

// ListLifecycleTransitions returns the audit trail for one strategy,
// newest first. limit 0 defaults to 50.
func (s *Store) ListLifecycleTransitions(ctx context.Context, strategyID string, limit int) ([]LifecycleTransition, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
SELECT id, strategy_id, from_stage, to_stage, kind, actor, reason, transitioned_ms
FROM strategy_lifecycle_transitions
WHERE strategy_id = ?
ORDER BY transitioned_ms DESC
LIMIT ?
`
	rows, err := s.db.QueryContext(ctx, q, strategyID, limit)
	if err != nil {
		return nil, fmt.Errorf("adminstore: list transitions: %w", err)
	}
	defer rows.Close()

	out := make([]LifecycleTransition, 0, limit)
	for rows.Next() {
		var (
			t      LifecycleTransition
			reason sql.NullString
		)
		if err := rows.Scan(&t.ID, &t.StrategyID, &t.FromStage, &t.ToStage,
			&t.Kind, &t.Actor, &reason, &t.TransitionedMS); err != nil {
			return nil, fmt.Errorf("adminstore: scan transition: %w", err)
		}
		t.Reason = reason.String
		out = append(out, t)
	}
	return out, rows.Err()
}

// LifecycleBoardRow is the flattened view returned by the board query —
// one row per strategy with enough detail for the Kanban card.
type LifecycleBoardRow struct {
	ID             int64
	StrategyID     string
	StrategyType   string
	Stage          string
	Status         string
	ConfigJSON     string
	UpdatedMS      int64
	LastTransition sql.NullInt64 // transitioned_ms of the most recent entry; may be NULL for never-moved drafts
}

// ListLifecycleBoard returns every strategy with its current stage and
// the timestamp of its most recent transition (NULL for never-moved
// drafts). The UI groups this by stage to render the Kanban board.
func (s *Store) ListLifecycleBoard(ctx context.Context) ([]LifecycleBoardRow, error) {
	const q = `
SELECT sc.id, sc.strategy_id, sc.strategy_type,
       COALESCE(sc.lifecycle_stage, 'draft'), sc.status, sc.config_json, sc.updated_ms,
       (SELECT MAX(transitioned_ms)
          FROM strategy_lifecycle_transitions lt
          WHERE lt.strategy_id = sc.strategy_id) AS last_transition
FROM strategy_configs sc
ORDER BY sc.id
`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("adminstore: list lifecycle board: %w", err)
	}
	defer rows.Close()

	out := make([]LifecycleBoardRow, 0, 32)
	for rows.Next() {
		var r LifecycleBoardRow
		if err := rows.Scan(&r.ID, &r.StrategyID, &r.StrategyType,
			&r.Stage, &r.Status, &r.ConfigJSON, &r.UpdatedMS, &r.LastTransition); err != nil {
			return nil, fmt.Errorf("adminstore: scan lifecycle row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
