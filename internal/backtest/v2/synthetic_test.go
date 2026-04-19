package v2

import (
	"math"
	"testing"
)

func TestGenerateSynthetic_EmptyWhenNumEventsNonPositive(t *testing.T) {
	ds := GenerateSynthetic(SyntheticConfig{Symbol: "X", NumEvents: 0})
	if len(ds.Events) != 0 {
		t.Fatalf("events=%d want 0", len(ds.Events))
	}
}

func TestGenerateSynthetic_DeterministicFromSeed(t *testing.T) {
	cfg := SyntheticConfig{
		Symbol:        "BTCUSDT",
		NumEvents:     50,
		Seed:          42,
		StartPrice:    20000,
		VolatilityBps: 15,
	}
	a := GenerateSynthetic(cfg)
	b := GenerateSynthetic(cfg)
	if len(a.Events) != len(b.Events) {
		t.Fatalf("lengths differ: %d vs %d", len(a.Events), len(b.Events))
	}
	for i := range a.Events {
		if a.Events[i] != b.Events[i] {
			t.Fatalf("event %d differs: %+v vs %+v", i, a.Events[i], b.Events[i])
		}
	}
}

func TestGenerateSynthetic_TrendDriftsPriceUpward(t *testing.T) {
	cfg := SyntheticConfig{
		Symbol:          "BTCUSDT",
		NumEvents:       500,
		Seed:            7,
		StartPrice:      100,
		VolatilityBps:   5,   // low noise
		TrendBpsPerStep: 10,  // 10bps/step upward drift
	}
	ds := GenerateSynthetic(cfg)
	last := ds.Events[len(ds.Events)-1].LastPX
	if last <= cfg.StartPrice {
		t.Fatalf("final px=%v did not drift above start %v", last, cfg.StartPrice)
	}
	// With +10bps drift × 500 steps, expected log drift ~ 0.5; price should be > 150.
	if last < 150 {
		t.Fatalf("trending drift too weak: last=%v want > 150", last)
	}
}

func TestGenerateSynthetic_FieldsPopulated(t *testing.T) {
	ds := GenerateSynthetic(SyntheticConfig{
		Symbol:    "BTCUSDT",
		NumEvents: 10,
		StartTSMS: 1_700_000_000_000,
		StepMS:    60_000,
		SpreadBps: 5,
		Seed:      1,
	})
	first := ds.Events[0]
	if first.Symbol != "BTCUSDT" {
		t.Fatalf("symbol=%q", first.Symbol)
	}
	if first.EmitTSMS != 1_700_000_000_000 {
		t.Fatalf("start ts=%d", first.EmitTSMS)
	}
	if first.BidPX >= first.LastPX || first.AskPX <= first.LastPX {
		t.Fatalf("spread wrong: bid=%v last=%v ask=%v", first.BidPX, first.LastPX, first.AskPX)
	}
	halfSpread := (first.AskPX - first.BidPX) / 2.0 / first.LastPX
	if math.Abs(halfSpread-0.0005) > 1e-9 {
		t.Fatalf("half spread=%v want 0.0005 (5bps)", halfSpread)
	}
	// Monotonic timestamps.
	for i := 1; i < len(ds.Events); i++ {
		if ds.Events[i].EmitTSMS <= ds.Events[i-1].EmitTSMS {
			t.Fatalf("ts not increasing at %d", i)
		}
	}
}
