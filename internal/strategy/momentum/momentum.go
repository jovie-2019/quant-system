package momentum

import (
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
	if !strings.EqualFold(evt.Symbol, s.cfg.Symbol) {
		return nil
	}
	if evt.LastPX <= 0 {
		return nil
	}

	startedAt := time.Now()
	outcome := "no_signal"
	defer func() {
		metrics.ObserveMomentumEvaluation(s.cfg.Symbol, outcome, time.Since(startedAt))
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Compute high/low from existing window BEFORE pushing new price.
	windowFull := s.count >= s.cfg.WindowSize
	var high, low float64
	if windowFull {
		high, low = s.windowHighLow()
	}

	// Push into ring buffer.
	s.ring[s.head] = evt.LastPX
	s.head = (s.head + 1) % s.cfg.WindowSize
	if s.count < s.cfg.WindowSize {
		s.count++
	}

	if !windowFull {
		outcome = "warming"
		return nil
	}
	now := time.Now()

	// Cooldown check.
	if !s.lastSig.IsZero() && now.Sub(s.lastSig) < s.cfg.Cooldown {
		outcome = "cooldown_skip"
		return nil
	}

	px := evt.LastPX
	intentTS := fmt.Sprintf("momentum-%s-%d-%d", s.cfg.Symbol, now.UnixMilli(), intentSeq.Add(1))

	if px > high*(1+s.cfg.BreakoutThreshold) {
		s.lastSig = now
		s.hasPos = true
		outcome = "buy_signal"
		metrics.ObserveMomentumSignal(s.cfg.Symbol, "buy")
		s.cfg.Logger.Info("momentum BUY signal",
			"symbol", s.cfg.Symbol,
			"price", px,
			"window_high", high,
			"intent_id", intentTS,
		)
		return []contracts.OrderIntent{{
			IntentID:    intentTS,
			Symbol:      s.cfg.Symbol,
			Side:        "buy",
			Price:       px,
			Quantity:    s.cfg.OrderQty,
			TimeInForce: s.cfg.TimeInForce,
		}}
	}

	if s.hasPos && px < low*(1-s.cfg.BreakoutThreshold) {
		s.lastSig = now
		s.hasPos = false
		outcome = "sell_signal"
		metrics.ObserveMomentumSignal(s.cfg.Symbol, "sell")
		s.cfg.Logger.Info("momentum SELL signal",
			"symbol", s.cfg.Symbol,
			"price", px,
			"window_low", low,
			"intent_id", intentTS,
		)
		return []contracts.OrderIntent{{
			IntentID:    intentTS,
			Symbol:      s.cfg.Symbol,
			Side:        "sell",
			Price:       px,
			Quantity:    s.cfg.OrderQty,
			TimeInForce: s.cfg.TimeInForce,
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
