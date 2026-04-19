package adminapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"

	v2 "quant-system/internal/backtest/v2"
)

// BacktestStatus is the lifecycle state of a backtest job.
type BacktestStatus string

const (
	// BacktestStatusQueued means the job has been accepted but execution
	// has not started yet. The MVP runs backtests synchronously so this
	// state is transient.
	BacktestStatusQueued BacktestStatus = "queued"
	// BacktestStatusRunning means the engine is replaying the dataset.
	BacktestStatusRunning BacktestStatus = "running"
	// BacktestStatusDone means the backtest completed successfully; Result is set.
	BacktestStatusDone BacktestStatus = "done"
	// BacktestStatusFailed means the backtest aborted; Error holds the reason.
	BacktestStatusFailed BacktestStatus = "failed"
)

// BacktestDatasetSpec describes the market data source for a backtest run.
// Supported Source values:
//   - "synthetic": use the fields below to generate a random walk.
//   - "clickhouse": use VenueName + IntervalName + [StartTSMS, EndTSMS]
//     to query the configured KlineStore; NumEvents acts as an optional cap.
type BacktestDatasetSpec struct {
	Source    string `json:"source"`
	Symbol    string `json:"symbol"`
	NumEvents int    `json:"num_events"`

	// Synthetic-only knobs.
	Seed            int64   `json:"seed,omitempty"`
	StartPrice      float64 `json:"start_price,omitempty"`
	VolatilityBps   float64 `json:"volatility_bps,omitempty"`
	TrendBpsPerStep float64 `json:"trend_bps_per_step,omitempty"`
	SpreadBps       float64 `json:"spread_bps,omitempty"`
	StepMS          int64   `json:"step_ms,omitempty"`

	// Shared: for synthetic this is the first event timestamp; for
	// clickhouse it is the inclusive lower bound of the query window.
	StartTSMS int64 `json:"start_ts_ms,omitempty"`
	// EndTSMSField is the inclusive upper bound of the query window for
	// ClickHouse-sourced datasets. Ignored for synthetic.
	EndTSMSField int64 `json:"end_ts_ms,omitempty"`

	// Venue / interval selectors for ClickHouse lookups. Ignored for synthetic.
	VenueName    string `json:"venue,omitempty"`
	IntervalName string `json:"interval,omitempty"`
}

// Interval returns the normalised interval string (e.g. "1m").
func (s BacktestDatasetSpec) Interval() string { return s.IntervalName }

// Venue returns the venue name used to scope ClickHouse queries.
func (s BacktestDatasetSpec) Venue() string { return s.VenueName }

// EndTSMS returns the inclusive upper bound of the query window.
func (s BacktestDatasetSpec) EndTSMS() int64 { return s.EndTSMSField }

// BacktestRiskSpec mirrors the risk.Config subset exposed to the UI.
type BacktestRiskSpec struct {
	MaxOrderQty    float64  `json:"max_order_qty"`
	MaxOrderAmount float64  `json:"max_order_amount"`
	AllowedSymbols []string `json:"allowed_symbols"`
}

// BacktestRequest is the body of POST /api/v1/backtests.
type BacktestRequest struct {
	StrategyType   string              `json:"strategy_type"`
	StrategyParams json.RawMessage     `json:"strategy_params"`
	Dataset        BacktestDatasetSpec `json:"dataset"`
	StartEquity    float64             `json:"start_equity"`
	SlippageBps    float64             `json:"slippage_bps"`
	FeeBps         float64             `json:"fee_bps"`
	Risk           BacktestRiskSpec    `json:"risk"`
	AccountID      string              `json:"account_id"`
}

// BacktestRecord is the in-memory representation of a backtest job.
type BacktestRecord struct {
	ID         string          `json:"id"`
	Status     BacktestStatus  `json:"status"`
	Error      string          `json:"error,omitempty"`
	Request    BacktestRequest `json:"request"`
	Result     *v2.Result      `json:"result,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  time.Time       `json:"started_at,omitempty"`
	FinishedAt time.Time       `json:"finished_at,omitempty"`
}

// Clone returns a deep-enough copy suitable for returning from the store
// without callers mutating stored state. Result is shared because it is
// large and treated as immutable once set.
func (r *BacktestRecord) Clone() *BacktestRecord {
	cp := *r
	return &cp
}

// BacktestStore is a bounded in-memory store of backtest records. Records
// are retained in insertion order; once the capacity is exceeded the oldest
// record is evicted. A future revision can back this with MySQL by
// implementing the same Put/Get/List signature.
type BacktestStore struct {
	mu      sync.RWMutex
	records map[string]*BacktestRecord
	order   []string
	max     int
}

// NewBacktestStore returns a store that retains the most recent `max`
// records. A non-positive max is clamped to 100.
func NewBacktestStore(max int) *BacktestStore {
	if max <= 0 {
		max = 100
	}
	return &BacktestStore{
		records: make(map[string]*BacktestRecord, max),
		order:   make([]string, 0, max),
		max:     max,
	}
}

// Put inserts or replaces a record.
func (s *BacktestStore) Put(rec *BacktestRecord) {
	if rec == nil || rec.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.records[rec.ID]; !exists {
		s.order = append(s.order, rec.ID)
		if len(s.order) > s.max {
			evict := s.order[0]
			s.order = s.order[1:]
			delete(s.records, evict)
		}
	}
	s.records[rec.ID] = rec
}

// Get returns a clone of the record with the given ID if it exists.
func (s *BacktestStore) Get(id string) (*BacktestRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	if !ok {
		return nil, false
	}
	return rec.Clone(), true
}

// Update mutates a stored record in place under the store lock. Returns
// false if the record does not exist.
func (s *BacktestStore) Update(id string, mutate func(*BacktestRecord)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return false
	}
	mutate(rec)
	return true
}

// List returns clones of the most recent records, newest first. A limit of 0
// or negative returns all retained records.
func (s *BacktestStore) List(limit int) []*BacktestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*BacktestRecord, 0, len(s.order))
	for _, id := range s.order {
		if rec, ok := s.records[id]; ok {
			out = append(out, rec.Clone())
		}
	}
	// newest first by CreatedAt descending
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// newBacktestID returns a short opaque ID like "bt_<unix_ms>_<8hex>".
func newBacktestID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "bt_" + hex.EncodeToString(b) + "_" + time.Now().UTC().Format("20060102T150405")
}
