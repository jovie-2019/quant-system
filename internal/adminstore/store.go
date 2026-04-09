package adminstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNilDB is returned when a nil database handle is provided.
	ErrNilDB = errors.New("adminstore: db is nil")
)

// Exchange represents a configured trading exchange.
type Exchange struct {
	ID        int64
	Name      string
	Venue     string // "binance" or "okx"
	Status    string // "active" or "disabled"
	CreatedMS int64
	UpdatedMS int64
}

// APIKey represents an encrypted API key record for an exchange.
type APIKey struct {
	ID            int64
	ExchangeID    int64
	Label         string
	APIKeyEnc     string // encrypted
	APISecretEnc  string // encrypted
	PassphraseEnc string // encrypted
	Permissions   string
	Status        string
	CreatedMS     int64
	UpdatedMS     int64
}

// StrategyConfig represents a strategy configuration record.
type StrategyConfig struct {
	ID           int64
	StrategyID   string
	StrategyType string
	ExchangeID   int64
	APIKeyID     int64
	ConfigJSON   string
	Status       string // "running", "stopped"
	CreatedMS    int64
	UpdatedMS    int64
}

// Store provides admin data persistence backed by MySQL.
type Store struct {
	db *sql.DB
}

// NewStore creates a new admin Store with the given database handle.
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, ErrNilDB
	}
	return &Store{db: db}, nil
}

// DB returns the underlying database handle.
func (s *Store) DB() *sql.DB {
	return s.db
}

// EnsureSchema creates all admin tables if they do not exist.
func (s *Store) EnsureSchema(ctx context.Context) error {
	for _, stmt := range adminDDL {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("adminstore: ensure schema: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Exchanges
// ---------------------------------------------------------------------------

// CreateExchange inserts a new exchange and returns its ID.
func (s *Store) CreateExchange(ctx context.Context, ex Exchange) (int64, error) {
	now := time.Now().UnixMilli()
	const q = `
INSERT INTO exchanges (name, venue, status, created_ms, updated_ms)
VALUES (?, ?, ?, ?, ?)
`
	res, err := s.db.ExecContext(ctx, q, ex.Name, ex.Venue, ex.Status, now, now)
	if err != nil {
		return 0, fmt.Errorf("adminstore: create exchange: %w", err)
	}
	return res.LastInsertId()
}

// GetExchange retrieves an exchange by ID. The boolean indicates whether the row was found.
func (s *Store) GetExchange(ctx context.Context, id int64) (Exchange, bool, error) {
	const q = `
SELECT id, name, venue, status, created_ms, updated_ms
FROM exchanges WHERE id = ?
`
	var ex Exchange
	if err := s.db.QueryRowContext(ctx, q, id).Scan(
		&ex.ID, &ex.Name, &ex.Venue, &ex.Status, &ex.CreatedMS, &ex.UpdatedMS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Exchange{}, false, nil
		}
		return Exchange{}, false, fmt.Errorf("adminstore: get exchange: %w", err)
	}
	return ex, true, nil
}

// ListExchanges returns all exchanges.
func (s *Store) ListExchanges(ctx context.Context) ([]Exchange, error) {
	const q = `
SELECT id, name, venue, status, created_ms, updated_ms
FROM exchanges ORDER BY id
`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("adminstore: list exchanges: %w", err)
	}
	defer rows.Close()

	out := make([]Exchange, 0, 16)
	for rows.Next() {
		var ex Exchange
		if err := rows.Scan(
			&ex.ID, &ex.Name, &ex.Venue, &ex.Status, &ex.CreatedMS, &ex.UpdatedMS,
		); err != nil {
			return nil, fmt.Errorf("adminstore: list exchanges scan: %w", err)
		}
		out = append(out, ex)
	}
	return out, rows.Err()
}

// UpdateExchange updates an existing exchange by ID.
func (s *Store) UpdateExchange(ctx context.Context, ex Exchange) error {
	now := time.Now().UnixMilli()
	const q = `
UPDATE exchanges SET name = ?, venue = ?, status = ?, updated_ms = ?
WHERE id = ?
`
	_, err := s.db.ExecContext(ctx, q, ex.Name, ex.Venue, ex.Status, now, ex.ID)
	if err != nil {
		return fmt.Errorf("adminstore: update exchange: %w", err)
	}
	return nil
}

// DeleteExchange deletes an exchange by ID.
func (s *Store) DeleteExchange(ctx context.Context, id int64) error {
	const q = `DELETE FROM exchanges WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("adminstore: delete exchange: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

// CreateAPIKey inserts a new API key and returns its ID.
func (s *Store) CreateAPIKey(ctx context.Context, key APIKey) (int64, error) {
	now := time.Now().UnixMilli()
	const q = `
INSERT INTO api_keys (exchange_id, label, api_key_enc, api_secret_enc, passphrase_enc, permissions, status, created_ms, updated_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := s.db.ExecContext(ctx, q,
		key.ExchangeID, key.Label, key.APIKeyEnc, key.APISecretEnc,
		key.PassphraseEnc, key.Permissions, key.Status, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("adminstore: create api key: %w", err)
	}
	return res.LastInsertId()
}

// GetAPIKey retrieves an API key by ID. The boolean indicates whether the row was found.
func (s *Store) GetAPIKey(ctx context.Context, id int64) (APIKey, bool, error) {
	const q = `
SELECT id, exchange_id, label, api_key_enc, api_secret_enc, passphrase_enc, permissions, status, created_ms, updated_ms
FROM api_keys WHERE id = ?
`
	var key APIKey
	if err := s.db.QueryRowContext(ctx, q, id).Scan(
		&key.ID, &key.ExchangeID, &key.Label, &key.APIKeyEnc, &key.APISecretEnc,
		&key.PassphraseEnc, &key.Permissions, &key.Status, &key.CreatedMS, &key.UpdatedMS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIKey{}, false, nil
		}
		return APIKey{}, false, fmt.Errorf("adminstore: get api key: %w", err)
	}
	return key, true, nil
}

// ListAPIKeys returns all API keys.
func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	const q = `
SELECT id, exchange_id, label, api_key_enc, api_secret_enc, passphrase_enc, permissions, status, created_ms, updated_ms
FROM api_keys ORDER BY id
`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("adminstore: list api keys: %w", err)
	}
	defer rows.Close()

	out := make([]APIKey, 0, 16)
	for rows.Next() {
		var key APIKey
		if err := rows.Scan(
			&key.ID, &key.ExchangeID, &key.Label, &key.APIKeyEnc, &key.APISecretEnc,
			&key.PassphraseEnc, &key.Permissions, &key.Status, &key.CreatedMS, &key.UpdatedMS,
		); err != nil {
			return nil, fmt.Errorf("adminstore: list api keys scan: %w", err)
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

// ListAPIKeysByExchange returns all API keys for a given exchange.
func (s *Store) ListAPIKeysByExchange(ctx context.Context, exchangeID int64) ([]APIKey, error) {
	const q = `
SELECT id, exchange_id, label, api_key_enc, api_secret_enc, passphrase_enc, permissions, status, created_ms, updated_ms
FROM api_keys WHERE exchange_id = ? ORDER BY id
`
	rows, err := s.db.QueryContext(ctx, q, exchangeID)
	if err != nil {
		return nil, fmt.Errorf("adminstore: list api keys: %w", err)
	}
	defer rows.Close()

	out := make([]APIKey, 0, 8)
	for rows.Next() {
		var key APIKey
		if err := rows.Scan(
			&key.ID, &key.ExchangeID, &key.Label, &key.APIKeyEnc, &key.APISecretEnc,
			&key.PassphraseEnc, &key.Permissions, &key.Status, &key.CreatedMS, &key.UpdatedMS,
		); err != nil {
			return nil, fmt.Errorf("adminstore: list api keys scan: %w", err)
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

// DeleteAPIKey deletes an API key by ID.
func (s *Store) DeleteAPIKey(ctx context.Context, id int64) error {
	const q = `DELETE FROM api_keys WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("adminstore: delete api key: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Strategy Configs
// ---------------------------------------------------------------------------

// CreateStrategyConfig inserts a new strategy config and returns its ID.
func (s *Store) CreateStrategyConfig(ctx context.Context, cfg StrategyConfig) (int64, error) {
	now := time.Now().UnixMilli()
	const q = `
INSERT INTO strategy_configs (strategy_id, strategy_type, exchange_id, api_key_id, config_json, status, created_ms, updated_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`
	res, err := s.db.ExecContext(ctx, q,
		cfg.StrategyID, cfg.StrategyType, cfg.ExchangeID, cfg.APIKeyID,
		cfg.ConfigJSON, cfg.Status, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("adminstore: create strategy config: %w", err)
	}
	return res.LastInsertId()
}

// GetStrategyConfig retrieves a strategy config by ID.
func (s *Store) GetStrategyConfig(ctx context.Context, id int64) (StrategyConfig, bool, error) {
	const q = `
SELECT id, strategy_id, strategy_type, exchange_id, api_key_id, config_json, status, created_ms, updated_ms
FROM strategy_configs WHERE id = ?
`
	var cfg StrategyConfig
	if err := s.db.QueryRowContext(ctx, q, id).Scan(
		&cfg.ID, &cfg.StrategyID, &cfg.StrategyType, &cfg.ExchangeID, &cfg.APIKeyID,
		&cfg.ConfigJSON, &cfg.Status, &cfg.CreatedMS, &cfg.UpdatedMS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StrategyConfig{}, false, nil
		}
		return StrategyConfig{}, false, fmt.Errorf("adminstore: get strategy config: %w", err)
	}
	return cfg, true, nil
}

// GetStrategyConfigByStrategyID retrieves a strategy config by its strategy_id string.
func (s *Store) GetStrategyConfigByStrategyID(ctx context.Context, strategyID string) (StrategyConfig, bool, error) {
	const q = `
SELECT id, strategy_id, strategy_type, exchange_id, api_key_id, config_json, status, created_ms, updated_ms
FROM strategy_configs WHERE strategy_id = ?
`
	var cfg StrategyConfig
	if err := s.db.QueryRowContext(ctx, q, strategyID).Scan(
		&cfg.ID, &cfg.StrategyID, &cfg.StrategyType, &cfg.ExchangeID, &cfg.APIKeyID,
		&cfg.ConfigJSON, &cfg.Status, &cfg.CreatedMS, &cfg.UpdatedMS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StrategyConfig{}, false, nil
		}
		return StrategyConfig{}, false, fmt.Errorf("adminstore: get strategy config by strategy id: %w", err)
	}
	return cfg, true, nil
}

// ListStrategyConfigs returns all strategy configs.
func (s *Store) ListStrategyConfigs(ctx context.Context) ([]StrategyConfig, error) {
	const q = `
SELECT id, strategy_id, strategy_type, exchange_id, api_key_id, config_json, status, created_ms, updated_ms
FROM strategy_configs ORDER BY id
`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("adminstore: list strategy configs: %w", err)
	}
	defer rows.Close()

	out := make([]StrategyConfig, 0, 32)
	for rows.Next() {
		var cfg StrategyConfig
		if err := rows.Scan(
			&cfg.ID, &cfg.StrategyID, &cfg.StrategyType, &cfg.ExchangeID, &cfg.APIKeyID,
			&cfg.ConfigJSON, &cfg.Status, &cfg.CreatedMS, &cfg.UpdatedMS,
		); err != nil {
			return nil, fmt.Errorf("adminstore: list strategy configs scan: %w", err)
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// UpdateStrategyConfig updates an existing strategy config by ID.
func (s *Store) UpdateStrategyConfig(ctx context.Context, cfg StrategyConfig) error {
	now := time.Now().UnixMilli()
	const q = `
UPDATE strategy_configs
SET strategy_id = ?, strategy_type = ?, exchange_id = ?, api_key_id = ?, config_json = ?, status = ?, updated_ms = ?
WHERE id = ?
`
	_, err := s.db.ExecContext(ctx, q,
		cfg.StrategyID, cfg.StrategyType, cfg.ExchangeID, cfg.APIKeyID,
		cfg.ConfigJSON, cfg.Status, now, cfg.ID,
	)
	if err != nil {
		return fmt.Errorf("adminstore: update strategy config: %w", err)
	}
	return nil
}

// UpdateStrategyStatus updates only the status field of a strategy config.
func (s *Store) UpdateStrategyStatus(ctx context.Context, id int64, status string) error {
	now := time.Now().UnixMilli()
	const q = `UPDATE strategy_configs SET status = ?, updated_ms = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, status, now, id)
	if err != nil {
		return fmt.Errorf("adminstore: update strategy status: %w", err)
	}
	return nil
}

// DeleteStrategyConfig deletes a strategy config by ID.
func (s *Store) DeleteStrategyConfig(ctx context.Context, id int64) error {
	const q = `DELETE FROM strategy_configs WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("adminstore: delete strategy config: %w", err)
	}
	return nil
}
