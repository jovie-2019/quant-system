package marketstore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"quant-system/internal/regime"
)

// RegimeQuery selects a slice of regime records. Venue and Method are
// optional — empty matches all. StartMS/EndMS are inclusive; a zero bound
// is treated as "no bound on that side".
type RegimeQuery struct {
	Venue    string
	Symbol   string
	Interval string
	Method   regime.Method
	StartMS  int64
	EndMS    int64
	Limit    int
}

// Validate returns an error when the query is missing required fields.
func (q RegimeQuery) Validate() error {
	if strings.TrimSpace(q.Symbol) == "" {
		return errors.New("marketstore: regime query symbol required")
	}
	if strings.TrimSpace(q.Interval) == "" {
		return errors.New("marketstore: regime query interval required")
	}
	if q.EndMS > 0 && q.StartMS > q.EndMS {
		return errors.New("marketstore: start_ms > end_ms")
	}
	return nil
}

// RegimeMatrixKey selects one cell of a latest-regime matrix view.
type RegimeMatrixKey struct {
	Venue    string
	Symbol   string
	Interval string
	Method   regime.Method
}

// RegimeStore persists classifier output. The interface mirrors
// KlineStore so a single backing store (ClickHouse) can implement both.
type RegimeStore interface {
	UpsertRegimes(ctx context.Context, recs []regime.Record) error
	QueryRegimes(ctx context.Context, q RegimeQuery) ([]regime.Record, error)
	// LatestRegimes returns the most recent record for each requested key.
	// Missing keys are simply omitted from the output.
	LatestRegimes(ctx context.Context, keys []RegimeMatrixKey) ([]regime.Record, error)
}

// -------------------- Memory impl --------------------

// InMemoryRegimeStore is safe for concurrent use and intended for tests
// and dev mode. Records are kept sorted by BarTime per (venue, symbol,
// interval, method) key.
type InMemoryRegimeStore struct {
	mu   sync.RWMutex
	data map[string][]regime.Record
}

// NewInMemoryRegimeStore returns an empty ready-to-use store.
func NewInMemoryRegimeStore() *InMemoryRegimeStore {
	return &InMemoryRegimeStore{data: make(map[string][]regime.Record)}
}

func regimeKey(venue, symbol, interval string, method regime.Method) string {
	return NormaliseVenue(venue) + "|" + NormaliseSymbol(symbol) + "|" + NormaliseInterval(interval) + "|" + string(method)
}

// UpsertRegimes inserts or replaces records keyed by (venue, symbol,
// interval, method, bar_time).
func (s *InMemoryRegimeStore) UpsertRegimes(_ context.Context, recs []regime.Record) error {
	if len(recs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range recs {
		k := regimeKey(r.Venue, r.Symbol, r.Interval, r.Method)
		bucket := s.data[k]
		idx := sort.Search(len(bucket), func(i int) bool { return bucket[i].BarTime >= r.BarTime })
		r.Symbol = NormaliseSymbol(r.Symbol)
		r.Interval = NormaliseInterval(r.Interval)
		if idx < len(bucket) && bucket[idx].BarTime == r.BarTime {
			bucket[idx] = r
		} else {
			bucket = append(bucket, regime.Record{})
			copy(bucket[idx+1:], bucket[idx:])
			bucket[idx] = r
		}
		s.data[k] = bucket
	}
	return nil
}

// QueryRegimes returns records matching the query, ordered by BarTime ASC.
func (s *InMemoryRegimeStore) QueryRegimes(_ context.Context, q RegimeQuery) ([]regime.Record, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]regime.Record, 0, 128)
	for key, bucket := range s.data {
		venue, symbol, interval, method := splitRegimeKey(key)
		if symbol != NormaliseSymbol(q.Symbol) || interval != NormaliseInterval(q.Interval) {
			continue
		}
		if q.Venue != "" && venue != NormaliseVenue(q.Venue) {
			continue
		}
		if q.Method != "" && method != string(q.Method) {
			continue
		}
		for _, r := range bucket {
			if q.StartMS > 0 && r.BarTime < q.StartMS {
				continue
			}
			if q.EndMS > 0 && r.BarTime > q.EndMS {
				break
			}
			out = append(out, r)
			if q.Limit > 0 && len(out) >= q.Limit {
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BarTime < out[j].BarTime })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// LatestRegimes returns the most recent Record per key.
func (s *InMemoryRegimeStore) LatestRegimes(_ context.Context, keys []RegimeMatrixKey) ([]regime.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]regime.Record, 0, len(keys))
	for _, k := range keys {
		if k.Method == "" {
			k.Method = regime.MethodThreshold
		}
		bucket := s.data[regimeKey(k.Venue, k.Symbol, k.Interval, k.Method)]
		if len(bucket) == 0 {
			continue
		}
		out = append(out, bucket[len(bucket)-1])
	}
	return out, nil
}

func splitRegimeKey(key string) (venue, symbol, interval, method string) {
	parts := strings.SplitN(key, "|", 4)
	for len(parts) < 4 {
		parts = append(parts, "")
	}
	return parts[0], parts[1], parts[2], parts[3]
}

// -------------------- ClickHouse impl --------------------

// UpsertRegimes writes records into quant.regime_history. The table is a
// ReplacingMergeTree so re-runs on the same (venue, symbol, interval,
// bar_time, method) tuple are safe.
func (s *ClickHouseStore) UpsertRegimes(ctx context.Context, recs []regime.Record) error {
	if len(recs) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO regime_history (
		  venue, symbol, interval, bar_time, method, regime, confidence,
		  adx, atr, bbw, hurst
		)
	`)
	if err != nil {
		return fmt.Errorf("marketstore/clickhouse: regime batch: %w", err)
	}
	for _, r := range recs {
		if err := batch.Append(
			NormaliseVenue(r.Venue),
			NormaliseSymbol(r.Symbol),
			NormaliseInterval(r.Interval),
			time.UnixMilli(r.BarTime),
			string(r.Method),
			string(r.Regime),
			float32(r.Confidence),
			float32(r.Features.ADX),
			float32(r.Features.ATR),
			float32(r.Features.BBW),
			float32(r.Features.Hurst),
		); err != nil {
			return fmt.Errorf("marketstore/clickhouse: regime append: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("marketstore/clickhouse: regime send: %w", err)
	}
	return nil
}

// QueryRegimes returns ordered records from quant.regime_history.
func (s *ClickHouseStore) QueryRegimes(ctx context.Context, q RegimeQuery) ([]regime.Record, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	sql := `
		SELECT venue, symbol, interval, bar_time, method, regime, confidence,
		       adx, atr, bbw, hurst
		FROM regime_history FINAL
		WHERE symbol = ? AND interval = ?`
	args := []any{NormaliseSymbol(q.Symbol), NormaliseInterval(q.Interval)}
	if q.Venue != "" {
		sql += ` AND venue = ?`
		args = append(args, NormaliseVenue(q.Venue))
	}
	if q.Method != "" {
		sql += ` AND method = ?`
		args = append(args, string(q.Method))
	}
	if q.StartMS > 0 {
		sql += ` AND bar_time >= ?`
		args = append(args, time.UnixMilli(q.StartMS))
	}
	if q.EndMS > 0 {
		sql += ` AND bar_time <= ?`
		args = append(args, time.UnixMilli(q.EndMS))
	}
	sql += ` ORDER BY bar_time ASC`
	if q.Limit > 0 {
		sql += fmt.Sprintf(` LIMIT %d`, q.Limit)
	}
	return scanRegimeRows(ctx, s.conn, sql, args)
}

// LatestRegimes returns one most-recent record per requested key. A
// single query with argMax is used to keep this cheap even when the caller
// asks for a large matrix.
func (s *ClickHouseStore) LatestRegimes(ctx context.Context, keys []RegimeMatrixKey) ([]regime.Record, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	// We build a compact IN-list of (venue, symbol, interval, method) tuples.
	type bin struct {
		venue, symbol, interval, method string
	}
	bins := make([]bin, 0, len(keys))
	for _, k := range keys {
		method := string(k.Method)
		if method == "" {
			method = string(regime.MethodThreshold)
		}
		bins = append(bins, bin{
			venue:    NormaliseVenue(k.Venue),
			symbol:   NormaliseSymbol(k.Symbol),
			interval: NormaliseInterval(k.Interval),
			method:   method,
		})
	}

	// One row per (venue, symbol, interval, method) with the latest bar.
	// argMax(col, bar_time) picks col at the row with the max bar_time.
	var wheres []string
	var args []any
	for _, b := range bins {
		wheres = append(wheres, `(venue = ? AND symbol = ? AND interval = ? AND method = ?)`)
		args = append(args, b.venue, b.symbol, b.interval, b.method)
	}
	sql := `
		SELECT venue, symbol, interval,
		       argMax(bar_time, bar_time) AS bar_time,
		       method,
		       argMax(regime, bar_time) AS regime,
		       argMax(confidence, bar_time) AS confidence,
		       argMax(adx, bar_time) AS adx,
		       argMax(atr, bar_time) AS atr,
		       argMax(bbw, bar_time) AS bbw,
		       argMax(hurst, bar_time) AS hurst
		FROM regime_history FINAL
		WHERE ` + strings.Join(wheres, " OR ") + `
		GROUP BY venue, symbol, interval, method`
	return scanRegimeRows(ctx, s.conn, sql, args)
}

func scanRegimeRows(ctx context.Context, conn driver.Conn, sql string, args []any) ([]regime.Record, error) {
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("marketstore/clickhouse: regime query: %w", err)
	}
	defer rows.Close()

	out := make([]regime.Record, 0, 256)
	for rows.Next() {
		var (
			venue, symbol, interval, method, regimeLabel string
			barTime                                      time.Time
			confidence, adx, atr, bbw, hurst             float32
		)
		if err := rows.Scan(&venue, &symbol, &interval, &barTime, &method, &regimeLabel, &confidence, &adx, &atr, &bbw, &hurst); err != nil {
			return nil, fmt.Errorf("marketstore/clickhouse: regime scan: %w", err)
		}
		out = append(out, regime.Record{
			Venue:      venue,
			Symbol:     symbol,
			Interval:   interval,
			BarTime:    barTime.UnixMilli(),
			Method:     regime.Method(method),
			Regime:     regime.Regime(regimeLabel),
			Confidence: float64(confidence),
			Features: regime.Features{
				ADX:   float64(adx),
				ATR:   float64(atr),
				BBW:   float64(bbw),
				Hurst: float64(hurst),
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("marketstore/clickhouse: regime rows: %w", err)
	}
	return out, nil
}
