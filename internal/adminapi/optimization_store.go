package adminapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"

	v2 "quant-system/internal/backtest/v2"
	"quant-system/internal/optimizer"
)

// OptimizationStatus mirrors BacktestStatus but is kept distinct so the
// UI can evolve each lifecycle independently (optimisations will gain
// "paused" / "cancelled" states once a job queue lands).
type OptimizationStatus string

const (
	OptimizationStatusQueued  OptimizationStatus = "queued"
	OptimizationStatusRunning OptimizationStatus = "running"
	OptimizationStatusDone    OptimizationStatus = "done"
	OptimizationStatusFailed  OptimizationStatus = "failed"
)

// OptimizationRequest is the body of POST /api/v1/optimizations.
// Mirrors optimizer.Config but uses JSON-friendly field names and
// embeds a BacktestDatasetSpec so the caller can reuse the same
// synthetic / clickhouse surface as the Backtest Workbench.
type OptimizationRequest struct {
	StrategyType string              `json:"strategy_type"`
	BaseParams   json.RawMessage     `json:"base_params"`
	Params       []OptParamPayload   `json:"params"`
	Dataset      BacktestDatasetSpec `json:"dataset"`
	StartEquity  float64             `json:"start_equity"`
	SlippageBps  float64             `json:"slippage_bps"`
	FeeBps       float64             `json:"fee_bps"`
	Risk         BacktestRiskSpec    `json:"risk"`

	Algorithm optimizer.Algorithm       `json:"algorithm,omitempty"`
	MaxTrials int                       `json:"max_trials,omitempty"`
	Seed      int64                     `json:"seed,omitempty"`
	Objective optimizer.ObjectivePreset `json:"objective,omitempty"`

	AccountID string `json:"account_id,omitempty"`
}

// OptParamPayload is the wire form of an optimizer.ParamSpec. Using its
// own shape (rather than reusing ParamSpec directly) keeps the REST
// surface stable if internal enums drift.
type OptParamPayload struct {
	Name     string             `json:"name"`
	Type     optimizer.ParamType `json:"type"`
	Min      float64            `json:"min,omitempty"`
	Max      float64            `json:"max,omitempty"`
	Step     float64            `json:"step,omitempty"`
	LogScale bool               `json:"log_scale,omitempty"`
	Choices  []any              `json:"choices,omitempty"`
}

// toSpec converts a payload param into the internal type.
func (p OptParamPayload) toSpec() optimizer.ParamSpec {
	return optimizer.ParamSpec{
		Name: p.Name, Type: p.Type, Min: p.Min, Max: p.Max,
		Step: p.Step, LogScale: p.LogScale, Choices: p.Choices,
	}
}

// OptimizationRecord is the in-memory representation of an optimisation job.
type OptimizationRecord struct {
	ID         string              `json:"id"`
	Status     OptimizationStatus  `json:"status"`
	Error      string              `json:"error,omitempty"`
	Request    OptimizationRequest `json:"request"`
	Result     *optimizer.Result   `json:"result,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
	StartedAt  time.Time           `json:"started_at,omitempty"`
	FinishedAt time.Time           `json:"finished_at,omitempty"`
}

// Clone returns a safe-to-return shallow copy.
func (r *OptimizationRecord) Clone() *OptimizationRecord {
	cp := *r
	return &cp
}

// OptimizationStore is a bounded in-memory job registry, mirroring
// BacktestStore exactly. See backtest_store.go for the rationale.
type OptimizationStore struct {
	mu      sync.RWMutex
	records map[string]*OptimizationRecord
	order   []string
	max     int
}

// NewOptimizationStore returns a store that retains the most recent
// `max` records. A non-positive max is clamped to 50.
func NewOptimizationStore(max int) *OptimizationStore {
	if max <= 0 {
		max = 50
	}
	return &OptimizationStore{
		records: make(map[string]*OptimizationRecord, max),
		order:   make([]string, 0, max),
		max:     max,
	}
}

// Put inserts or replaces a record. Evicts the oldest when at capacity.
func (s *OptimizationStore) Put(rec *OptimizationRecord) {
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

// Get returns a clone of the record with the given ID.
func (s *OptimizationStore) Get(id string) (*OptimizationRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	if !ok {
		return nil, false
	}
	return rec.Clone(), true
}

// Update mutates a stored record under the lock.
func (s *OptimizationStore) Update(id string, mutate func(*OptimizationRecord)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return false
	}
	mutate(rec)
	return true
}

// List returns clones sorted newest-first.
func (s *OptimizationStore) List(limit int) []*OptimizationRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*OptimizationRecord, 0, len(s.order))
	for _, id := range s.order {
		if rec, ok := s.records[id]; ok {
			out = append(out, rec.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func newOptimizationID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "opt_" + hex.EncodeToString(b) + "_" + time.Now().UTC().Format("20060102T150405")
}

// Silence linter when Result type changes shape.
var _ = v2.Result{}
