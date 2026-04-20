package optimizer

import (
	"encoding/json"
	"math/rand"
	"testing"
)

func TestParamSpec_Validate(t *testing.T) {
	cases := []struct {
		name string
		p    ParamSpec
		ok   bool
	}{
		{"no name", ParamSpec{Type: ParamInt, Min: 0, Max: 5}, false},
		{"bad bounds", ParamSpec{Name: "x", Type: ParamFloat, Min: 5, Max: 5}, false},
		{"log with zero min", ParamSpec{Name: "x", Type: ParamFloat, Min: 0, Max: 1, LogScale: true}, false},
		{"missing choices", ParamSpec{Name: "x", Type: ParamCategorical}, false},
		{"unknown type", ParamSpec{Name: "x", Type: "weird", Min: 0, Max: 1}, false},
		{"good int", ParamSpec{Name: "x", Type: ParamInt, Min: 1, Max: 10, Step: 1}, true},
		{"good cat", ParamSpec{Name: "x", Type: ParamCategorical, Choices: []any{"a", "b"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestSearchSpace_Validate(t *testing.T) {
	s := SearchSpace{StrategyType: "x", Params: []ParamSpec{
		{Name: "a", Type: ParamInt, Min: 1, Max: 3},
		{Name: "a", Type: ParamInt, Min: 1, Max: 3}, // duplicate
	}}
	if err := s.Validate(); err == nil {
		t.Fatal("expected duplicate error")
	}

	if err := (SearchSpace{}).Validate(); err == nil {
		t.Fatal("expected missing strategy_type error")
	}
	if err := (SearchSpace{StrategyType: "x"}).Validate(); err == nil {
		t.Fatal("expected no-params error")
	}
}

func TestParamSpec_GridInt(t *testing.T) {
	p := ParamSpec{Name: "n", Type: ParamInt, Min: 1, Max: 5, Step: 1}
	g := p.Grid()
	if len(g) != 5 {
		t.Fatalf("grid=%+v want 5 items", g)
	}
	if g[0].(int64) != 1 || g[len(g)-1].(int64) != 5 {
		t.Fatalf("bounds wrong: %+v", g)
	}
}

func TestParamSpec_GridFloatLog(t *testing.T) {
	p := ParamSpec{Name: "x", Type: ParamFloat, Min: 1e-3, Max: 1, LogScale: true}
	g := p.Grid()
	if len(g) < 3 {
		t.Fatalf("grid=%+v want multiple log steps", g)
	}
	if g[0].(float64) != 1e-3 {
		t.Fatalf("first=%v want 1e-3", g[0])
	}
	if last := g[len(g)-1].(float64); last < 0.9 || last > 1.0001 {
		t.Fatalf("last=%v want ~1", last)
	}
}

func TestParamSpec_GridCategorical(t *testing.T) {
	p := ParamSpec{Name: "mode", Type: ParamCategorical, Choices: []any{"a", "b", "c"}}
	g := p.Grid()
	if len(g) != 3 {
		t.Fatalf("grid=%+v want 3", g)
	}
}

func TestSample_Deterministic(t *testing.T) {
	p := ParamSpec{Name: "x", Type: ParamFloat, Min: 0, Max: 100}
	rng := rand.New(rand.NewSource(7))
	a := p.Sample(rng)
	rng = rand.New(rand.NewSource(7))
	b := p.Sample(rng)
	if a != b {
		t.Fatalf("seeded samples should match: %v vs %v", a, b)
	}
}

func TestEnumerateGrid_CartesianProduct(t *testing.T) {
	space := SearchSpace{
		StrategyType: "x",
		Params: []ParamSpec{
			{Name: "a", Type: ParamInt, Min: 1, Max: 3, Step: 1},
			{Name: "b", Type: ParamCategorical, Choices: []any{"x", "y"}},
		},
	}
	out := enumerateGrid(space)
	if len(out) != 6 {
		t.Fatalf("enumerated=%d want 6 (3*2)", len(out))
	}
}

func TestMergeParams(t *testing.T) {
	base := json.RawMessage(`{"symbol":"BTCUSDT","cooldown_ms":0}`)
	trial := map[string]any{"window_size": int64(20), "cooldown_ms": int64(500)}
	merged, err := mergeParams(base, trial)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(merged, &m)
	if m["symbol"] != "BTCUSDT" {
		t.Fatalf("symbol lost: %+v", m)
	}
	// Trial should override base.
	if int64(m["cooldown_ms"].(float64)) != 500 {
		t.Fatalf("override failed: %+v", m)
	}
	if int64(m["window_size"].(float64)) != 20 {
		t.Fatalf("window_size missing: %+v", m)
	}
}
