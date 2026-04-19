package v2

import (
	"math"
	"math/rand"

	"quant-system/pkg/contracts"
)

// SyntheticConfig parameters a deterministic random-walk market-event stream
// used by backtests before a real historical data source is attached.
//
// The price process is a geometric random walk: log-returns are drawn i.i.d.
// Normal(mu, sigma^2) where mu = TrendBpsPerStep / 10_000 and sigma =
// VolatilityBps / 10_000. BidPX/AskPX are symmetrically placed around LastPX
// using SpreadBps as the half-spread.
type SyntheticConfig struct {
	Symbol string // required
	Venue  string // default "sim"

	StartPrice float64 // default 100
	NumEvents  int     // required (>0)
	Seed       int64   // deterministic stream; zero picks 1
	StepMS     int64   // default 60_000 (1 minute between events)
	StartTSMS  int64   // default 1_735_689_600_000 (2025-01-01T00:00:00Z)

	VolatilityBps   float64 // per-step log-return std dev; default 20
	TrendBpsPerStep float64 // per-step log-return mean; default 0 (random walk)
	SpreadBps       float64 // half bid/ask spread; default 2
}

// GenerateSynthetic returns a deterministic Dataset ready to feed into Run.
// The output is fully reproducible for a given Seed, so tests and UI demos
// can reference specific "scenario" seeds (e.g. "trending up", "choppy").
func GenerateSynthetic(cfg SyntheticConfig) Dataset {
	if cfg.NumEvents <= 0 {
		return Dataset{Name: "synthetic-empty"}
	}
	if cfg.Symbol == "" {
		cfg.Symbol = "SIMUSDT"
	}
	if cfg.Venue == "" {
		cfg.Venue = "sim"
	}
	if cfg.StartPrice <= 0 {
		cfg.StartPrice = 100
	}
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}
	if cfg.StepMS <= 0 {
		cfg.StepMS = 60_000
	}
	if cfg.StartTSMS <= 0 {
		cfg.StartTSMS = 1_735_689_600_000
	}
	if cfg.VolatilityBps <= 0 {
		cfg.VolatilityBps = 20
	}
	if cfg.SpreadBps <= 0 {
		cfg.SpreadBps = 2
	}

	rng := rand.New(rand.NewSource(cfg.Seed))
	mu := cfg.TrendBpsPerStep / 10_000.0
	sigma := cfg.VolatilityBps / 10_000.0
	halfSpread := cfg.SpreadBps / 10_000.0

	events := make([]contracts.MarketNormalizedEvent, cfg.NumEvents)
	px := cfg.StartPrice
	ts := cfg.StartTSMS

	for i := 0; i < cfg.NumEvents; i++ {
		if i > 0 {
			// log-return = mu + sigma * N(0,1)
			ret := mu + sigma*rng.NormFloat64()
			px *= math.Exp(ret)
			if px < 1e-9 {
				px = 1e-9
			}
			ts += cfg.StepMS
		}
		events[i] = contracts.MarketNormalizedEvent{
			Venue:      contracts.Venue(cfg.Venue),
			Symbol:     cfg.Symbol,
			Sequence:   int64(i),
			BidPX:      px * (1 - halfSpread),
			BidSZ:      1,
			AskPX:      px * (1 + halfSpread),
			AskSZ:      1,
			LastPX:     px,
			SourceTSMS: ts,
			IngestTSMS: ts,
			EmitTSMS:   ts,
		}
	}
	return Dataset{Name: "synthetic", Events: events}
}
