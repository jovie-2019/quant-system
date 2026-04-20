// Package optimizer searches a strategy's parameter space by repeatedly
// invoking the backtest engine (internal/backtest/v2) and scoring each
// trial with a configurable objective function.
//
// Two algorithms ship in the MVP:
//
//   - "grid":    exhaustive enumeration of the Cartesian product of per-
//                parameter discretisations. Deterministic and predictable.
//   - "random":  uniformly sampled trials (seeded for reproducibility).
//                Scales better when the space is wide.
//
// A future Python sidecar will add Bayesian optimisation (Optuna) and
// tree-Parzen estimators; when that lands, it will produce Trial records
// with the same shape so consumers (REST, UI, persistence) need no
// changes.
package optimizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

// ParamType discriminates the allowed representations of a searchable
// parameter. "int" and "float" use [Min, Max]; "categorical" picks from
// Choices.
type ParamType string

const (
	ParamInt         ParamType = "int"
	ParamFloat       ParamType = "float"
	ParamCategorical ParamType = "categorical"
)

// ParamSpec describes one searchable parameter. For grid search both Min
// and Max are required along with either Step (linear) or count derived
// from the Choices slice for categoricals. Random search uses the same
// bounds but ignores Step.
type ParamSpec struct {
	Name     string       `json:"name"`
	Type     ParamType    `json:"type"`
	Min      float64      `json:"min,omitempty"`
	Max      float64      `json:"max,omitempty"`
	Step     float64      `json:"step,omitempty"`
	LogScale bool         `json:"log_scale,omitempty"`
	Choices  []any        `json:"choices,omitempty"`
}

// Validate returns an actionable error if the spec is malformed.
func (p ParamSpec) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("optimizer: param name required")
	}
	switch p.Type {
	case ParamInt, ParamFloat:
		if p.Min >= p.Max {
			return fmt.Errorf("optimizer: param %q: min (%v) must be < max (%v)", p.Name, p.Min, p.Max)
		}
		if p.LogScale && (p.Min <= 0 || p.Max <= 0) {
			return fmt.Errorf("optimizer: param %q: log_scale requires positive bounds", p.Name)
		}
	case ParamCategorical:
		if len(p.Choices) == 0 {
			return fmt.Errorf("optimizer: param %q: categorical requires at least one choice", p.Name)
		}
	default:
		return fmt.Errorf("optimizer: param %q: unknown type %q", p.Name, p.Type)
	}
	return nil
}

// SearchSpace groups the searchable parameters with the fixed strategy
// fields (StrategyType + BaseParams) that apply to every trial.
type SearchSpace struct {
	StrategyType string          `json:"strategy_type"`
	BaseParams   json.RawMessage `json:"base_params"`
	Params       []ParamSpec     `json:"params"`
}

// Validate checks the search space is well-formed.
func (s SearchSpace) Validate() error {
	if strings.TrimSpace(s.StrategyType) == "" {
		return errors.New("optimizer: strategy_type is required")
	}
	if len(s.Params) == 0 {
		return errors.New("optimizer: at least one search parameter required")
	}
	names := make(map[string]struct{}, len(s.Params))
	for _, p := range s.Params {
		if err := p.Validate(); err != nil {
			return err
		}
		if _, dup := names[p.Name]; dup {
			return fmt.Errorf("optimizer: duplicate param %q", p.Name)
		}
		names[p.Name] = struct{}{}
	}
	return nil
}

// Grid returns the ordered discretisation of a numeric ParamSpec. For
// categoricals it returns Choices unchanged. For float/int with missing
// Step the function picks a step that yields ~10 samples across the range.
func (p ParamSpec) Grid() []any {
	switch p.Type {
	case ParamCategorical:
		out := make([]any, len(p.Choices))
		copy(out, p.Choices)
		return out
	case ParamInt:
		step := p.Step
		if step <= 0 {
			step = math.Max(1, math.Round((p.Max-p.Min)/9))
		}
		out := make([]any, 0)
		for v := p.Min; v <= p.Max+1e-9; v += step {
			out = append(out, int64(math.Round(v)))
		}
		return dedupe(out)
	case ParamFloat:
		step := p.Step
		if step <= 0 {
			step = (p.Max - p.Min) / 9
		}
		out := make([]any, 0)
		if p.LogScale {
			// Geometric spacing.
			steps := int(math.Round((math.Log(p.Max) - math.Log(p.Min)) / math.Max(math.Log1p(step/math.Max(p.Min, 1e-9)), 1e-9)))
			if steps < 2 {
				steps = 9
			}
			for i := 0; i <= steps; i++ {
				t := float64(i) / float64(steps)
				v := p.Min * math.Pow(p.Max/p.Min, t)
				out = append(out, v)
			}
		} else {
			for v := p.Min; v <= p.Max+1e-12; v += step {
				out = append(out, v)
			}
		}
		return dedupe(out)
	}
	return nil
}

// Sample returns a single random value for the parameter. The rng is
// owned by the caller so multi-param sampling is deterministic when a
// seeded rng is passed in.
func (p ParamSpec) Sample(rng *rand.Rand) any {
	switch p.Type {
	case ParamCategorical:
		return p.Choices[rng.Intn(len(p.Choices))]
	case ParamInt:
		v := p.Min + rng.Float64()*(p.Max-p.Min)
		return int64(math.Round(v))
	case ParamFloat:
		if p.LogScale {
			t := rng.Float64()
			return p.Min * math.Pow(p.Max/p.Min, t)
		}
		return p.Min + rng.Float64()*(p.Max-p.Min)
	}
	return nil
}

// enumerateGrid returns every combination of per-param grids as full
// parameter maps. Callers bound the result size via SearchSpace
// validation and the optimizer's MaxTrials cap.
func enumerateGrid(space SearchSpace) []map[string]any {
	grids := make([][]any, len(space.Params))
	for i, p := range space.Params {
		grids[i] = p.Grid()
		if len(grids[i]) == 0 {
			return nil
		}
	}
	// Iterative cartesian product to avoid recursion depth for wide spaces.
	total := 1
	for _, g := range grids {
		total *= len(g)
	}
	out := make([]map[string]any, 0, total)
	idx := make([]int, len(grids))
	for {
		m := make(map[string]any, len(space.Params))
		for i, p := range space.Params {
			m[p.Name] = grids[i][idx[i]]
		}
		out = append(out, m)
		// Increment mixed-radix counter.
		pos := len(idx) - 1
		for pos >= 0 {
			idx[pos]++
			if idx[pos] < len(grids[pos]) {
				break
			}
			idx[pos] = 0
			pos--
		}
		if pos < 0 {
			break
		}
	}
	return out
}

// dedupe removes consecutive near-duplicates from a []any whose elements
// are numeric; used to smooth over float-step accumulation artifacts.
func dedupe(xs []any) []any {
	if len(xs) < 2 {
		return xs
	}
	out := xs[:1]
	prev, _ := toFloat(xs[0])
	for _, v := range xs[1:] {
		cur, ok := toFloat(v)
		if !ok || math.Abs(cur-prev) > 1e-12 {
			out = append(out, v)
			prev = cur
		}
	}
	return out
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// mergeParams merges base (fixed) params and trial (searched) params into
// a single JSON-encoded object suitable for handing to a strategy
// constructor. Trial keys override base keys on collision.
func mergeParams(base json.RawMessage, trial map[string]any) (json.RawMessage, error) {
	merged := map[string]any{}
	if len(base) > 0 {
		if err := json.Unmarshal(base, &merged); err != nil {
			return nil, fmt.Errorf("optimizer: decode base_params: %w", err)
		}
	}
	for k, v := range trial {
		merged[k] = v
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("optimizer: encode merged params: %w", err)
	}
	return raw, nil
}

// sortedParamNames returns a stable ordering for reproducible trial IDs
// and deterministic parallel-coordinate chart axes.
func sortedParamNames(space SearchSpace) []string {
	names := make([]string, 0, len(space.Params))
	for _, p := range space.Params {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}
