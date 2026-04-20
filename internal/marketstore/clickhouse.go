package marketstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"quant-system/pkg/contracts"
)

// ClickHouseConfig holds the connection parameters for ClickHouseStore.
type ClickHouseConfig struct {
	// Addrs is the list of host:port endpoints. For a single-node docker
	// deployment this is typically []string{"127.0.0.1:9000"}.
	Addrs []string
	// Database name (e.g. "quant"). Required.
	Database string
	// Username / Password as configured on the server.
	Username string
	Password string
	// DialTimeout governs the initial TCP/TLS handshake; defaults to 5s.
	DialTimeout time.Duration
	// ReadTimeout governs per-query reads; defaults to 30s.
	ReadTimeout time.Duration
}

// ClickHouseStore is the production KlineStore backed by a ClickHouse
// ReplacingMergeTree table. See deploy/clickhouse/init.sql for the schema.
type ClickHouseStore struct {
	conn driver.Conn
	cfg  ClickHouseConfig
}

// NewClickHouseStore opens a connection pool and verifies reachability.
// It does NOT create the schema; deploy/clickhouse/init.sql is mounted into
// the container init dir and runs on first boot.
func NewClickHouseStore(ctx context.Context, cfg ClickHouseConfig) (*ClickHouseStore, error) {
	if len(cfg.Addrs) == 0 {
		return nil, errors.New("marketstore/clickhouse: addrs is required")
	}
	if cfg.Database == "" {
		return nil, errors.New("marketstore/clickhouse: database is required")
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: cfg.Addrs,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout: cfg.DialTimeout,
		ReadTimeout: cfg.ReadTimeout,
		Compression: &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	})
	if err != nil {
		return nil, fmt.Errorf("marketstore/clickhouse: open: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("marketstore/clickhouse: ping: %w", err)
	}
	return &ClickHouseStore{conn: conn, cfg: cfg}, nil
}

// Upsert batch-inserts klines via the native async batch API. The target
// table is a ReplacingMergeTree so repeated rows with the same natural key
// are deduplicated by ClickHouse at merge time; callers can retry without
// fear of producing duplicates at query time (older rows will be discarded
// at background merges).
func (s *ClickHouseStore) Upsert(ctx context.Context, klines []contracts.Kline) error {
	if len(klines) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO klines (venue, symbol, interval, open_time, close_time, open, high, low, close, volume)
	`)
	if err != nil {
		return fmt.Errorf("marketstore/clickhouse: prepare batch: %w", err)
	}
	for _, k := range klines {
		if err := batch.Append(
			string(k.Venue),
			NormaliseSymbol(k.Symbol),
			NormaliseInterval(k.Interval),
			time.UnixMilli(k.OpenTime),
			time.UnixMilli(k.CloseTime),
			k.Open,
			k.High,
			k.Low,
			k.Close,
			k.Volume,
		); err != nil {
			return fmt.Errorf("marketstore/clickhouse: append: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("marketstore/clickhouse: send batch: %w", err)
	}
	return nil
}

// Query returns klines matching the query, ordered by open_time ASC. The
// FINAL modifier is used so callers see deduplicated rows immediately even
// before ClickHouse has compacted the ReplacingMergeTree.
func (s *ClickHouseStore) Query(ctx context.Context, q KlineQuery) ([]contracts.Kline, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	sql, args := buildSelectSQL(q)
	rows, err := s.conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("marketstore/clickhouse: query: %w", err)
	}
	defer rows.Close()

	out := make([]contracts.Kline, 0, 512)
	for rows.Next() {
		var (
			venue, symbol, interval string
			openTime, closeTime     time.Time
			open, high, low, cls, vol float64
		)
		if err := rows.Scan(&venue, &symbol, &interval, &openTime, &closeTime, &open, &high, &low, &cls, &vol); err != nil {
			return nil, fmt.Errorf("marketstore/clickhouse: scan: %w", err)
		}
		out = append(out, contracts.Kline{
			Venue:     contracts.Venue(venue),
			Symbol:    symbol,
			Interval:  interval,
			OpenTime:  openTime.UnixMilli(),
			CloseTime: closeTime.UnixMilli(),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     cls,
			Volume:    vol,
			Closed:    true,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("marketstore/clickhouse: rows err: %w", err)
	}
	return out, nil
}

// Count returns the number of deduplicated rows matching the query.
func (s *ClickHouseStore) Count(ctx context.Context, q KlineQuery) (int64, error) {
	if err := q.Validate(); err != nil {
		return 0, err
	}
	sql, args := buildCountSQL(q)
	var n uint64
	if err := s.conn.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("marketstore/clickhouse: count: %w", err)
	}
	return int64(n), nil
}

// Ping delegates to the driver.
func (s *ClickHouseStore) Ping(ctx context.Context) error { return s.conn.Ping(ctx) }

// Close closes the underlying connection pool.
func (s *ClickHouseStore) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func buildSelectSQL(q KlineQuery) (string, []any) {
	sql := `
		SELECT venue, symbol, interval, open_time, close_time, open, high, low, close, volume
		FROM klines FINAL
		WHERE symbol = ? AND interval = ?`
	args := []any{NormaliseSymbol(q.Symbol), NormaliseInterval(q.Interval)}
	if q.Venue != "" {
		sql += ` AND venue = ?`
		args = append(args, NormaliseVenue(q.Venue))
	}
	if q.StartMS > 0 {
		sql += ` AND open_time >= ?`
		args = append(args, time.UnixMilli(q.StartMS))
	}
	if q.EndMS > 0 {
		sql += ` AND open_time <= ?`
		args = append(args, time.UnixMilli(q.EndMS))
	}
	sql += ` ORDER BY open_time ASC`
	if q.Limit > 0 {
		sql += fmt.Sprintf(` LIMIT %d`, q.Limit)
	}
	return sql, args
}

func buildCountSQL(q KlineQuery) (string, []any) {
	sql := `
		SELECT count() FROM klines FINAL
		WHERE symbol = ? AND interval = ?`
	args := []any{NormaliseSymbol(q.Symbol), NormaliseInterval(q.Interval)}
	if q.Venue != "" {
		sql += ` AND venue = ?`
		args = append(args, NormaliseVenue(q.Venue))
	}
	if q.StartMS > 0 {
		sql += ` AND open_time >= ?`
		args = append(args, time.UnixMilli(q.StartMS))
	}
	if q.EndMS > 0 {
		sql += ` AND open_time <= ?`
		args = append(args, time.UnixMilli(q.EndMS))
	}
	return sql, args
}
