package momentum

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"quant-system/internal/obs/metrics"
	"quant-system/pkg/contracts"
)

var intentSeq atomic.Int64

type Config struct {
	Symbol            string
	WindowSize        int
	BreakoutThreshold float64
	OrderQty          float64
	TimeInForce       string
	Cooldown          time.Duration
	Logger            *slog.Logger
}

func (c *Config) defaults() {
	if c.WindowSize <= 0 {
		c.WindowSize = 20
	}
	if c.BreakoutThreshold <= 0 {
		c.BreakoutThreshold = 0.001
	}
	if c.TimeInForce == "" {
		c.TimeInForce = "IOC"
	}
	if c.Cooldown < 0 {
		c.Cooldown = 5 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Strategy implements the momentum breakout strategy.
type Strategy struct {
	mu      sync.Mutex
	cfg     Config
	ring    []float64
	count   int
	head    int
	hasPos  bool
	lastSig time.Time
}

// New creates a momentum breakout Strategy.
func New(cfg Config) *Strategy {
	cfg.defaults()
	return &Strategy{
		cfg:  cfg,
		ring: make([]float64, cfg.WindowSize),
	}
}

func (s *Strategy) ID() string { return "momentum-breakout" }

func (s *Strategy) OnMarket(evt contracts.MarketNormalizedEvent) []contracts.OrderIntent {
	if evt.LastPX <= 0 {
		return nil
	}
	startedAt := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// All cfg access lives under the mutex so ApplyParams can race-free
	// swap fields from another goroutine. Snapshot the values we need so
	// the metric observer captures a local, not a moving field.
	cfg := s.cfg
	outcome := "no_signal"
	defer func() {
		metrics.ObserveMomentumEvaluation(cfg.Symbol, outcome, time.Since(startedAt))
	}()

	if !strings.EqualFold(evt.Symbol, cfg.Symbol) {
		return nil
	}

	// Compute high/low from existing window BEFORE pushing new price.
	windowFull := s.count >= cfg.WindowSize
	var high, low float64
	if windowFull {
		high, low = s.windowHighLow()
	}

	// Push into ring buffer.
	s.ring[s.head] = evt.LastPX
	s.head = (s.head + 1) % cfg.WindowSize
	if s.count < cfg.WindowSize {
		s.count++
	}

	if !windowFull {
		outcome = "warming"
		return nil
	}
	now := time.Now()

	// Cooldown check.
	if !s.lastSig.IsZero() && now.Sub(s.lastSig) < cfg.Cooldown {
		outcome = "cooldown_skip"
		return nil
	}

	px := evt.LastPX
	intentTS := fmt.Sprintf("momentum-%s-%d-%d", cfg.Symbol, now.UnixMilli(), intentSeq.Add(1))

	if px > high*(1+cfg.BreakoutThreshold) {
		s.lastSig = now
		s.hasPos = true
		outcome = "buy_signal"
		metrics.ObserveMomentumSignal(cfg.Symbol, "buy")
		cfg.Logger.Info("momentum BUY signal",
			"symbol", cfg.Symbol,
			"price", px,
			"window_high", high,
			"intent_id", intentTS,
		)
		return []contracts.OrderIntent{{
			IntentID:    intentTS,
			Symbol:      cfg.Symbol,
			Side:        "buy",
			Price:       px,
			Quantity:    cfg.OrderQty,
			TimeInForce: cfg.TimeInForce,
		}}
	}

	if s.hasPos && px < low*(1-cfg.BreakoutThreshold) {
		s.lastSig = now
		s.hasPos = false
		outcome = "sell_signal"
		metrics.ObserveMomentumSignal(cfg.Symbol, "sell")
		cfg.Logger.Info("momentum SELL signal",
			"symbol", cfg.Symbol,
			"price", px,
			"window_low", low,
			"intent_id", intentTS,
		)
		return []contracts.OrderIntent{{
			IntentID:    intentTS,
			Symbol:      cfg.Symbol,
			Side:        "sell",
			Price:       px,
			Quantity:    cfg.OrderQty,
			TimeInForce: cfg.TimeInForce,
		}}
	}

	return nil
}

func (s *Strategy) windowHighLow() (float64, float64) {
	high := -math.MaxFloat64
	low := math.MaxFloat64
	for _, v := range s.ring {
		if v > high {
			high = v
		}
		if v < low {
			low = v
		}
	}
	return high, low
}

// HasPosition returns whether the strategy believes it is in a position (for testing).
func (s *Strategy) HasPosition() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasPos
}

// CurrentConfig returns a copy of the currently active configuration.
// Useful for diagnostics and for constructing before/after diffs in the
// audit log when an operator proposes a new parameter set.
func (s *Strategy) CurrentConfig() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// paramReloadPayload is the JSON shape accepted by ApplyParams. It mirrors
// the fields a Config is typically serialised with (CooldownMS is exposed
// rather than a time.Duration so the wire format matches the configurator
// contract used elsewhere).
type paramReloadPayload struct {
	Symbol            string  `json:"symbol"`
	WindowSize        int     `json:"window_size"`
	BreakoutThreshold float64 `json:"breakout_threshold"`
	OrderQty          float64 `json:"order_qty"`
	TimeInForce       string  `json:"time_in_force"`
	CooldownMS        int     `json:"cooldown_ms"`
}

// ApplyParams hot-swaps the strategy's configuration without rebuilding
// the instance. Internal state — ring buffer contents, position flag,
// cooldown timestamp — is preserved whenever possible:
//
//   - WindowSize change: the ring is resized, which loses warm-up; we
//     copy as much of the tail of the old ring as fits so the strategy
//     can keep producing signals without waiting a full new window.
//   - Symbol change: rejected. A symbol swap is effectively a new
//     strategy and should go through the full Replace() path.
//   - All other fields: swapped in place.
//
// ApplyParams implements strategy.ParamReloader.
func (s *Strategy) ApplyParams(raw json.RawMessage) error {
	var p paramReloadPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("momentum: invalid params: %w", err)
	}
	if strings.TrimSpace(p.Symbol) == "" {
		return errors.New("momentum: symbol is required")
	}
	if p.WindowSize <= 0 {
		p.WindowSize = 20
	}
	if p.OrderQty <= 0 {
		return errors.New("momentum: order_qty must be > 0")
	}
	if p.BreakoutThreshold <= 0 {
		p.BreakoutThreshold = 0.001
	}

	newCfg := Config{
		Symbol:            p.Symbol,
		WindowSize:        p.WindowSize,
		BreakoutThreshold: p.BreakoutThreshold,
		OrderQty:          p.OrderQty,
		TimeInForce:       p.TimeInForce,
		Cooldown:          time.Duration(p.CooldownMS) * time.Millisecond,
	}
	newCfg.defaults()

	s.mu.Lock()
	defer s.mu.Unlock()

	if !strings.EqualFold(newCfg.Symbol, s.cfg.Symbol) {
		return fmt.Errorf("momentum: symbol swap from %q to %q is not hot-reloadable; use full replace",
			s.cfg.Symbol, newCfg.Symbol)
	}

	if newCfg.WindowSize != s.cfg.WindowSize {
		// Resize ring, preserving the *newest* tail of existing data. The
		// circular buffer stores samples at positions
		//   head-count .. head-1   (mod L)
		// where `head` is the index the next write will go to. When
		// shrinking to K, we copy positions head-K .. head-1 (mod L) in
		// chronological order into newRing[0..K-1]. When growing, we
		// copy all `count` samples starting at newRing[0].
		newRing := make([]float64, newCfg.WindowSize)
		if s.count > 0 {
			take := s.count
			if take > newCfg.WindowSize {
				take = newCfg.WindowSize
			}
			L := len(s.ring)
			for i := 0; i < take; i++ {
				srcIdx := (s.head - take + i + L*2) % L
				newRing[i] = s.ring[srcIdx]
			}
			s.count = take
			s.head = take % newCfg.WindowSize
		} else {
			s.head = 0
			s.count = 0
		}
		s.ring = newRing
	}
	s.cfg = newCfg
	return nil
}
