// Package marketstore defines the KlineStore interface used by the backtest
// engine, the kline-backfill CLI, and (future) the regime-detector service.
// The in-memory implementation is for tests and local development; the
// ClickHouse implementation is the production-grade durable store. A future
// DuckDB/Parquet implementation can slot in behind the same interface for
// single-workstation deployments without ClickHouse.
package marketstore

import (
	"context"
	"errors"
	"strings"

	"quant-system/pkg/contracts"
)

// ErrInvalidQuery is returned when a KlineQuery has incompatible parameters
// (e.g. StartMS > EndMS, missing Symbol/Interval).
var ErrInvalidQuery = errors.New("marketstore: invalid query")

// KlineQuery selects a slice of klines for retrieval. StartMS/EndMS are
// inclusive. A zero Limit means "no limit"; otherwise results are truncated
// to the earliest N matching rows.
type KlineQuery struct {
	Venue    string
	Symbol   string
	Interval string
	StartMS  int64
	EndMS    int64
	Limit    int
}

// Validate returns ErrInvalidQuery with a descriptive message if the query
// is malformed.
func (q KlineQuery) Validate() error {
	if strings.TrimSpace(q.Symbol) == "" {
		return errors.New("marketstore: symbol is required")
	}
	if strings.TrimSpace(q.Interval) == "" {
		return errors.New("marketstore: interval is required")
	}
	if q.EndMS > 0 && q.StartMS > q.EndMS {
		return errors.New("marketstore: start_ms > end_ms")
	}
	if q.Limit < 0 {
		return errors.New("marketstore: limit must be >= 0")
	}
	return nil
}

// KlineStore is the abstraction exposed to callers. Implementations must be
// goroutine-safe: backtests and the backfill CLI frequently call Upsert and
// Query concurrently with different (symbol, interval) slices.
type KlineStore interface {
	// Upsert saves or overwrites klines. It is idempotent on the natural key
	// (venue, symbol, interval, open_time) — a repeated call with the same
	// key and different values replaces the existing row.
	Upsert(ctx context.Context, klines []contracts.Kline) error

	// Query returns klines within [StartMS, EndMS] ordered by OpenTime ASC.
	Query(ctx context.Context, q KlineQuery) ([]contracts.Kline, error)

	// Count returns the number of rows matching the query (ignoring Limit).
	// It is cheap for backfill progress display and for gap detection.
	Count(ctx context.Context, q KlineQuery) (int64, error)

	// Ping validates connectivity. Implementations without a backing service
	// may return nil unconditionally.
	Ping(ctx context.Context) error

	// Close releases resources. Calling Close on an already-closed store
	// must be a no-op.
	Close() error
}

// NormaliseSymbol returns the canonical upper-case, trimmed form used as a
// storage key. Call sites that accept user input should always pass through
// this helper so queries and writes align.
func NormaliseSymbol(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// NormaliseInterval returns the canonical lower-case, trimmed form
// (1m, 5m, 1h, 1d, …).
func NormaliseInterval(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormaliseVenue returns the canonical lower-case venue name. Empty input
// yields the empty string so callers can treat "no filter" consistently.
func NormaliseVenue(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
