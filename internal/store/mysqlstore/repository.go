package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"quant-system/pkg/contracts"
)

var (
	ErrNilDB = errors.New("mysqlstore: db is nil")
)

type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type Repository struct {
	db *sql.DB
}

func Open(cfg Config) (*sql.DB, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, fmt.Errorf("mysqlstore: dsn is empty")
	}
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	return db, nil
}

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &Repository{db: db}, nil
}

func (r *Repository) EnsureSchema(ctx context.Context) error {
	for _, stmt := range schemaDDL {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) UpsertOrder(ctx context.Context, order contracts.Order) error {
	const q = `
INSERT INTO orders (
	client_order_id, venue_order_id, symbol, state, filled_qty, avg_price, state_version, updated_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	venue_order_id = VALUES(venue_order_id),
	symbol = VALUES(symbol),
	state = VALUES(state),
	filled_qty = VALUES(filled_qty),
	avg_price = VALUES(avg_price),
	state_version = VALUES(state_version),
	updated_ms = VALUES(updated_ms)
`
	_, err := r.db.ExecContext(ctx, q,
		order.ClientOrderID,
		order.VenueOrderID,
		strings.ToUpper(strings.TrimSpace(order.Symbol)),
		string(order.State),
		order.FilledQty,
		order.AvgPrice,
		order.StateVersion,
		order.UpdatedMS,
	)
	return err
}

func (r *Repository) GetOrder(ctx context.Context, clientOrderID string) (contracts.Order, bool, error) {
	const q = `
SELECT client_order_id, venue_order_id, symbol, state, filled_qty, avg_price, state_version, updated_ms
FROM orders WHERE client_order_id = ?
`
	row := r.db.QueryRowContext(ctx, q, clientOrderID)

	var (
		order contracts.Order
		state string
	)
	if err := row.Scan(
		&order.ClientOrderID,
		&order.VenueOrderID,
		&order.Symbol,
		&state,
		&order.FilledQty,
		&order.AvgPrice,
		&order.StateVersion,
		&order.UpdatedMS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contracts.Order{}, false, nil
		}
		return contracts.Order{}, false, err
	}
	order.State = contracts.OrderState(state)
	return order, true, nil
}

func (r *Repository) LoadOrders(ctx context.Context) ([]contracts.Order, error) {
	const q = `
SELECT client_order_id, venue_order_id, symbol, state, filled_qty, avg_price, state_version, updated_ms
FROM orders
`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]contracts.Order, 0, 128)
	for rows.Next() {
		var (
			order contracts.Order
			state string
		)
		if err := rows.Scan(
			&order.ClientOrderID,
			&order.VenueOrderID,
			&order.Symbol,
			&state,
			&order.FilledQty,
			&order.AvgPrice,
			&order.StateVersion,
			&order.UpdatedMS,
		); err != nil {
			return nil, err
		}
		order.State = contracts.OrderState(state)
		out = append(out, order)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertPosition(ctx context.Context, snapshot contracts.PositionSnapshot) error {
	const q = `
INSERT INTO positions (
	account_id, symbol, quantity, avg_cost, realized_pnl, updated_ms
) VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	quantity = VALUES(quantity),
	avg_cost = VALUES(avg_cost),
	realized_pnl = VALUES(realized_pnl),
	updated_ms = VALUES(updated_ms)
`
	_, err := r.db.ExecContext(ctx, q,
		snapshot.AccountID,
		strings.ToUpper(strings.TrimSpace(snapshot.Symbol)),
		snapshot.Quantity,
		snapshot.AvgCost,
		snapshot.RealizedPnL,
		snapshot.UpdatedMS,
	)
	return err
}

func (r *Repository) GetPosition(ctx context.Context, accountID, symbol string) (contracts.PositionSnapshot, bool, error) {
	const q = `
SELECT account_id, symbol, quantity, avg_cost, realized_pnl, updated_ms
FROM positions WHERE account_id = ? AND symbol = ?
`
	row := r.db.QueryRowContext(ctx, q, accountID, strings.ToUpper(strings.TrimSpace(symbol)))

	var snapshot contracts.PositionSnapshot
	if err := row.Scan(
		&snapshot.AccountID,
		&snapshot.Symbol,
		&snapshot.Quantity,
		&snapshot.AvgCost,
		&snapshot.RealizedPnL,
		&snapshot.UpdatedMS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contracts.PositionSnapshot{}, false, nil
		}
		return contracts.PositionSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (r *Repository) LoadPositions(ctx context.Context) ([]contracts.PositionSnapshot, error) {
	const q = `
SELECT account_id, symbol, quantity, avg_cost, realized_pnl, updated_ms
FROM positions
`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]contracts.PositionSnapshot, 0, 128)
	for rows.Next() {
		var snapshot contracts.PositionSnapshot
		if err := rows.Scan(
			&snapshot.AccountID,
			&snapshot.Symbol,
			&snapshot.Quantity,
			&snapshot.AvgCost,
			&snapshot.RealizedPnL,
			&snapshot.UpdatedMS,
		); err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	return out, rows.Err()
}

func (r *Repository) SaveRiskDecision(ctx context.Context, decision contracts.RiskDecision) error {
	const q = `
INSERT INTO risk_decisions (
	intent_id, strategy_id, symbol, side, price, quantity, decision, rule_id, reason_code, evaluated_ms, updated_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	strategy_id = VALUES(strategy_id),
	symbol = VALUES(symbol),
	side = VALUES(side),
	price = VALUES(price),
	quantity = VALUES(quantity),
	decision = VALUES(decision),
	rule_id = VALUES(rule_id),
	reason_code = VALUES(reason_code),
	evaluated_ms = VALUES(evaluated_ms),
	updated_ms = VALUES(updated_ms)
`
	_, err := r.db.ExecContext(ctx, q,
		decision.Intent.IntentID,
		decision.Intent.StrategyID,
		strings.ToUpper(strings.TrimSpace(decision.Intent.Symbol)),
		strings.ToLower(strings.TrimSpace(decision.Intent.Side)),
		decision.Intent.Price,
		decision.Intent.Quantity,
		string(decision.Decision),
		decision.RuleID,
		decision.ReasonCode,
		decision.EvaluatedMS,
		time.Now().UnixMilli(),
	)
	return err
}

func (r *Repository) GetRiskDecision(ctx context.Context, intentID string) (contracts.RiskDecision, bool, error) {
	const q = `
SELECT intent_id, strategy_id, symbol, side, price, quantity, decision, rule_id, reason_code, evaluated_ms
FROM risk_decisions WHERE intent_id = ?
`
	row := r.db.QueryRowContext(ctx, q, intentID)

	var out contracts.RiskDecision
	var decision string
	if err := row.Scan(
		&out.Intent.IntentID,
		&out.Intent.StrategyID,
		&out.Intent.Symbol,
		&out.Intent.Side,
		&out.Intent.Price,
		&out.Intent.Quantity,
		&decision,
		&out.RuleID,
		&out.ReasonCode,
		&out.EvaluatedMS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contracts.RiskDecision{}, false, nil
		}
		return contracts.RiskDecision{}, false, err
	}
	out.Decision = contracts.RiskDecisionType(decision)
	return out, true, nil
}
