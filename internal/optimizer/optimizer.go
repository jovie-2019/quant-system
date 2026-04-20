package optimizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	v2 "quant-system/internal/backtest/v2"
	"quant-system/internal/risk"
	"quant-system/internal/strategy"
)

// Algorithm enumerates the supported search strategies. "grid" is
// exhaustive, "random" samples uniformly with a seeded RNG. Both write
// Trial records with the same shape.
type Algorithm string

const (
	AlgorithmGrid   Algorithm = "grid"
	AlgorithmRandom Algorithm = "random"
)

// Config controls a single optimisation run. Zero values take sensible
// defaults.
type Config struct {
	Space SearchSpace

	// Algorithm selects the search strategy. Empty defaults to "grid"
	// when the enumerated space is ≤ MaxTrials, otherwise "random".
	Algorithm Algorithm

	// MaxTrials caps the number of backtests executed. For grid search
	// this also truncates the enumeration deterministically.
	MaxTrials int

	// Dataset is the single event stream evaluated per trial. Callers
	// constructing in-process synthetic or historical datasets should
	// reuse one Dataset across the run so trials are directly comparable.
	Dataset v2.Dataset

	// AccountID, StartEquity, Risk, Matcher mirror v2.Config. They apply
	// to every trial.
	AccountID   string
	StartEquity float64
	Risk        risk.Config
	Matcher     v2.SimMatcherConfig

	// Objective scores each completed backtest; higher wins. Nil
	// defaults to ObjectiveSharpePenaltyDD.
	Objective ObjectiveFn

	// Seed makes random search reproducible. Zero falls back to 1 so
	// independent calls with default values still yield the same output.
	Seed int64
}

// Trial records one completed backtest + its score.
type Trial struct {
	ID         int                `json:"id"`
	Params     map[string]any     `json:"params"`
	Objective  float64            `json:"objective"`
	Metrics    v2.Metrics         `json:"metrics"`
	Error      string             `json:"error,omitempty"`
	DurationMS int64              `json:"duration_ms"`
	BuildError string             `json:"build_error,omitempty"`
}

// Result is the full output of an optimisation run.
type Result struct {
	Trials      []Trial            `json:"trials"`
	Best        Trial              `json:"best"`
	Algorithm   Algorithm          `json:"algorithm"`
	Objective   ObjectivePreset    `json:"objective,omitempty"` // set by REST layer
	Importance  map[string]float64 `json:"importance"`
	Stability   float64            `json:"stability"` // 0..1; 1 means best params are robust to ±20% neighbours
	StartedAt   time.Time          `json:"started_at"`
	FinishedAt  time.Time          `json:"finished_at"`
	DurationMS  int64              `json:"duration_ms"`
	Strategy    string             `json:"strategy_type"`
	DatasetName string             `json:"dataset_name"`
}

// ErrEmptySpace is returned when the search space has no parameters.
var ErrEmptySpace = errors.New("optimizer: empty search space")

// ErrEmptyDataset is returned when no events are supplied.
var ErrEmptyDataset = errors.New("optimizer: empty dataset")

// Run executes an optimisation synchronously. For long-running cases
// callers should wrap this call in a goroutine and expose status via a
// job store, exactly the pattern the adminapi layer uses.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	if err := cfg.Space.Validate(); err != nil {
		return nil, err
	}
	if len(cfg.Dataset.Events) == 0 {
		return nil, ErrEmptyDataset
	}
	if cfg.MaxTrials <= 0 {
		cfg.MaxTrials = 200
	}
	if cfg.Objective == nil {
		cfg.Objective = NewObjective(ObjectiveSharpePenaltyDD)
	}
	if cfg.StartEquity <= 0 {
		cfg.StartEquity = 10_000
	}
	if cfg.AccountID == "" {
		cfg.AccountID = "opt-default"
	}
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}

	paramSets := plan(cfg)
	if len(paramSets) == 0 {
		return nil, ErrEmptySpace
	}

	startedAt := time.Now()
	trials := make([]Trial, 0, len(paramSets))
	for i, p := range paramSets {
		select {
		case <-ctx.Done():
			return partialResult(cfg, trials, startedAt), ctx.Err()
		default:
		}
		trials = append(trials, evaluate(ctx, cfg, i, p))
	}

	best := pickBest(trials)
	res := &Result{
		Trials:      trials,
		Best:        best,
		Algorithm:   resolveAlgorithm(cfg, len(paramSets)),
		Importance:  importance(trials, cfg.Space),
		Stability:   stability(ctx, cfg, best),
		StartedAt:   startedAt,
		FinishedAt:  time.Now(),
		Strategy:    cfg.Space.StrategyType,
		DatasetName: cfg.Dataset.Name,
	}
	res.DurationMS = res.FinishedAt.Sub(res.StartedAt).Milliseconds()
	return res, nil
}

// plan expands the search space into a bounded list of param maps
// according to the configured (or inferred) algorithm.
func plan(cfg Config) []map[string]any {
	algo := resolveAlgorithm(cfg, -1)
	switch algo {
	case AlgorithmGrid:
		all := enumerateGrid(cfg.Space)
		if cfg.MaxTrials > 0 && len(all) > cfg.MaxTrials {
			return all[:cfg.MaxTrials]
		}
		return all
	case AlgorithmRandom:
		rng := rand.New(rand.NewSource(cfg.Seed))
		out := make([]map[string]any, 0, cfg.MaxTrials)
		for i := 0; i < cfg.MaxTrials; i++ {
			m := map[string]any{}
			for _, p := range cfg.Space.Params {
				m[p.Name] = p.Sample(rng)
			}
			out = append(out, m)
		}
		return out
	}
	return nil
}

// resolveAlgorithm picks the effective algorithm, respecting an explicit
// Config.Algorithm and falling back to grid-when-small / random-otherwise.
func resolveAlgorithm(cfg Config, plannedCount int) Algorithm {
	if cfg.Algorithm != "" {
		return cfg.Algorithm
	}
	if plannedCount > 0 && plannedCount <= cfg.MaxTrials {
		return AlgorithmGrid
	}
	// Estimate grid cardinality cheaply; if < MaxTrials pick grid.
	total := 1
	for _, p := range cfg.Space.Params {
		g := len(p.Grid())
		if g <= 0 {
			g = 1
		}
		total *= g
		if total > cfg.MaxTrials && cfg.MaxTrials > 0 {
			return AlgorithmRandom
		}
	}
	return AlgorithmGrid
}

// evaluate runs one trial and packages the result. Errors in strategy
// construction or backtest execution are captured into the Trial, not
// returned — the optimiser must keep going on bad configs.
func evaluate(ctx context.Context, cfg Config, id int, params map[string]any) Trial {
	start := time.Now()
	trial := Trial{ID: id, Params: copyMap(params)}

	merged, err := mergeParams(cfg.Space.BaseParams, params)
	if err != nil {
		trial.BuildError = err.Error()
		trial.Objective = invalidScore
		trial.DurationMS = time.Since(start).Milliseconds()
		return trial
	}

	ctor, ok := strategy.Lookup(cfg.Space.StrategyType)
	if !ok {
		trial.BuildError = fmt.Sprintf("unknown strategy_type %q", cfg.Space.StrategyType)
		trial.Objective = invalidScore
		trial.DurationMS = time.Since(start).Milliseconds()
		return trial
	}
	strat, err := ctor(merged)
	if err != nil {
		trial.BuildError = fmt.Sprintf("build strategy: %v", err)
		trial.Objective = invalidScore
		trial.DurationMS = time.Since(start).Milliseconds()
		return trial
	}

	btCfg := v2.Config{
		AccountID:   cfg.AccountID,
		Strategy:    strat,
		Dataset:     cfg.Dataset,
		StartEquity: cfg.StartEquity,
		Risk:        cfg.Risk,
		Matcher:     cfg.Matcher,
	}
	res, err := v2.Run(ctx, btCfg)
	trial.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		trial.Error = err.Error()
		trial.Objective = invalidScore
		return trial
	}
	trial.Metrics = res.Metrics
	trial.Objective = cfg.Objective(res.Metrics)
	return trial
}

func pickBest(trials []Trial) Trial {
	if len(trials) == 0 {
		return Trial{Objective: invalidScore}
	}
	best := trials[0]
	for _, t := range trials[1:] {
		if t.Objective > best.Objective {
			best = t
		}
	}
	return best
}

// importance scores each parameter by the variance-of-objective
// explained when the parameter is the only axis varying. The score is
// normalised so all parameter importances sum to 1.
//
// For an MVP this is a quick-and-dirty "one-way ANOVA lite": for each
// param we compute the std-dev of objective scores binned by that
// param's value and divide by the overall std-dev. A tightly-clustered
// per-bin spread (low within-bin variance) with wide between-bin spread
// suggests the parameter matters. We collapse this to a single scalar
// per parameter — enough for a UI ranking without dragging in a stats
// library.
func importance(trials []Trial, space SearchSpace) map[string]float64 {
	out := make(map[string]float64, len(space.Params))
	if len(trials) < 2 {
		return out
	}
	// Collect finite-objective trials only.
	pool := make([]Trial, 0, len(trials))
	for _, t := range trials {
		if isFinite(t.Objective) && t.Objective > invalidScore/2 {
			pool = append(pool, t)
		}
	}
	if len(pool) < 2 {
		return out
	}
	totalStd := stdevObjective(pool)
	if totalStd <= 0 {
		return out
	}

	total := 0.0
	for _, p := range space.Params {
		bins := map[string][]Trial{}
		for _, t := range pool {
			key := fmt.Sprintf("%v", t.Params[p.Name])
			bins[key] = append(bins[key], t)
		}
		// Mean of per-bin means weighted by bin size.
		binMeans := make([]float64, 0, len(bins))
		for _, b := range bins {
			binMeans = append(binMeans, meanObjective(b))
		}
		if len(binMeans) < 2 {
			out[p.Name] = 0
			continue
		}
		between := stdev(binMeans) / totalStd
		out[p.Name] = between
		total += between
	}
	if total > 0 {
		for k, v := range out {
			out[k] = v / total
		}
	}
	return out
}

// stability measures how robust the best trial is to small perturbations
// of its parameters. We pick neighbours by moving each numeric parameter
// ±20% (keeping categoricals fixed) one at a time, evaluate them, and
// return min(neighbour_obj) / best_obj. A result ≥ 0.8 indicates a
// well-shaped peak; values near or below 0 indicate a razor-sharp
// overfit.
func stability(ctx context.Context, cfg Config, best Trial) float64 {
	if best.Objective <= invalidScore/2 {
		return 0
	}
	// Build neighbour param sets.
	neighbours := make([]map[string]any, 0, 2*len(cfg.Space.Params))
	for _, p := range cfg.Space.Params {
		if p.Type == ParamCategorical {
			continue
		}
		vRaw, ok := best.Params[p.Name]
		if !ok {
			continue
		}
		v, fin := toFloat(vRaw)
		if !fin || v == 0 {
			continue
		}
		for _, scale := range []float64{0.8, 1.2} {
			nv := v * scale
			if nv < p.Min {
				nv = p.Min
			}
			if nv > p.Max {
				nv = p.Max
			}
			neighbour := copyMap(best.Params)
			if p.Type == ParamInt {
				neighbour[p.Name] = int64(math.Round(nv))
			} else {
				neighbour[p.Name] = nv
			}
			neighbours = append(neighbours, neighbour)
		}
	}
	if len(neighbours) == 0 {
		return 1
	}
	worst := math.Inf(1)
	for _, p := range neighbours {
		select {
		case <-ctx.Done():
			return 0
		default:
		}
		t := evaluate(ctx, cfg, -1, p)
		if t.Objective < worst {
			worst = t.Objective
		}
	}
	if best.Objective <= 0 {
		// Bias toward 0 when the best objective is non-positive — we
		// cannot meaningfully form the ratio.
		return 0
	}
	ratio := worst / best.Objective
	if !isFinite(ratio) {
		return 0
	}
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return ratio
}

func partialResult(cfg Config, trials []Trial, started time.Time) *Result {
	best := pickBest(trials)
	return &Result{
		Trials:     trials,
		Best:       best,
		Algorithm:  resolveAlgorithm(cfg, len(trials)),
		Importance: importance(trials, cfg.Space),
		Stability:  0,
		StartedAt:  started,
		FinishedAt: time.Now(),
		DurationMS: time.Since(started).Milliseconds(),
		Strategy:   cfg.Space.StrategyType,
		DatasetName: cfg.Dataset.Name,
	}
}

// --- small helpers ---

func meanObjective(ts []Trial) float64 {
	if len(ts) == 0 {
		return 0
	}
	var s float64
	for _, t := range ts {
		s += t.Objective
	}
	return s / float64(len(ts))
}

func stdevObjective(ts []Trial) float64 {
	if len(ts) < 2 {
		return 0
	}
	m := meanObjective(ts)
	var sse float64
	for _, t := range ts {
		d := t.Objective - m
		sse += d * d
	}
	return math.Sqrt(sse / float64(len(ts)-1))
}

func stdev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var m float64
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	var sse float64
	for _, x := range xs {
		d := x - m
		sse += d * d
	}
	return math.Sqrt(sse / float64(len(xs)-1))
}

func copyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// SerializeParams emits params in a stable key order for human-readable
// display. Used by the REST layer when writing responses to the UI.
func SerializeParams(p map[string]any) json.RawMessage {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf := strings.Builder{}
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		vb, _ := json.Marshal(p[k])
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return json.RawMessage(buf.String())
}
