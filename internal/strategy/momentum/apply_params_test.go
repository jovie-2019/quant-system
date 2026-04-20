package momentum

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"quant-system/pkg/contracts"
)

// feedPrices is a test helper that walks a price series through the
// strategy and returns the count of emitted buy intents.
func feedPrices(s *Strategy, prices []float64) int {
	buys := 0
	for _, p := range prices {
		intents := s.OnMarket(contracts.MarketNormalizedEvent{
			Symbol: s.cfg.Symbol,
			LastPX: p,
		})
		for _, in := range intents {
			if in.Side == "buy" {
				buys++
			}
		}
	}
	return buys
}

func TestApplyParams_RejectsSymbolSwap(t *testing.T) {
	s := New(Config{Symbol: "BTCUSDT", WindowSize: 5, OrderQty: 1, Cooldown: 0})
	raw := []byte(`{"symbol":"ETHUSDT","window_size":5,"order_qty":1,"breakout_threshold":0.001}`)
	err := s.ApplyParams(raw)
	if err == nil || !strings.Contains(err.Error(), "symbol swap") {
		t.Fatalf("err=%v want symbol-swap rejection", err)
	}
}

func TestApplyParams_RequiresOrderQty(t *testing.T) {
	s := New(Config{Symbol: "BTCUSDT", WindowSize: 5, OrderQty: 1, Cooldown: 0})
	err := s.ApplyParams([]byte(`{"symbol":"BTCUSDT","window_size":5,"order_qty":0}`))
	if err == nil || !strings.Contains(err.Error(), "order_qty") {
		t.Fatalf("err=%v want order_qty rejection", err)
	}
}

func TestApplyParams_HotSwapsThreshold(t *testing.T) {
	s := New(Config{
		Symbol: "BTCUSDT", WindowSize: 5,
		BreakoutThreshold: 0.5, // very loose
		OrderQty:          1,
		Cooldown:          0,
	})

	// Swap to a much tighter threshold; the ring buffer state is preserved.
	newParams, _ := json.Marshal(paramReloadPayload{
		Symbol:            "BTCUSDT",
		WindowSize:        5,
		BreakoutThreshold: 0.000001, // tight enough that almost any move triggers
		OrderQty:          1,
		CooldownMS:        0,
	})
	if err := s.ApplyParams(newParams); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if s.CurrentConfig().BreakoutThreshold >= 0.5 {
		t.Fatalf("threshold not swapped: %+v", s.CurrentConfig())
	}
}

func TestApplyParams_ResizeWindowPreservesTail(t *testing.T) {
	s := New(Config{Symbol: "BTCUSDT", WindowSize: 10, OrderQty: 1, BreakoutThreshold: 0.001, Cooldown: 0})

	// Fill the window with increasing prices.
	prices := []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110}
	_ = feedPrices(s, prices)

	// Resize to a window of 5. The strategy should retain the last 5 samples.
	raw, _ := json.Marshal(paramReloadPayload{
		Symbol: "BTCUSDT", WindowSize: 5, OrderQty: 1, BreakoutThreshold: 0.001,
	})
	if err := s.ApplyParams(raw); err != nil {
		t.Fatal(err)
	}

	cfg := s.CurrentConfig()
	if cfg.WindowSize != 5 {
		t.Fatalf("window size=%d want 5", cfg.WindowSize)
	}

	s.mu.Lock()
	if s.count != 5 {
		t.Fatalf("count=%d want 5 after resize", s.count)
	}
	// Retained prices must be the last five in chronological order.
	want := []float64{106, 107, 108, 109, 110}
	got := make([]float64, 5)
	for i := 0; i < 5; i++ {
		idx := (s.head - s.count + i + len(s.ring)*2) % len(s.ring)
		got[i] = s.ring[idx]
	}
	s.mu.Unlock()

	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("tail[%d]=%v want %v", i, got[i], want[i])
		}
	}
}

func TestApplyParams_ConcurrentWithOnMarketIsSafe(t *testing.T) {
	s := New(Config{Symbol: "BTCUSDT", WindowSize: 20, OrderQty: 1, BreakoutThreshold: 0.001, Cooldown: 0})

	// Warm the window up so OnMarket does real work.
	for i := 0; i < 40; i++ {
		s.OnMarket(contracts.MarketNormalizedEvent{Symbol: "BTCUSDT", LastPX: 100 + float64(i)})
	}

	// Use a close-once channel as the shared stop signal — time.After
	// returns a channel a SINGLE goroutine could drain, starving the
	// others. Closing `done` unblocks all receivers simultaneously.
	done := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(done)
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				s.OnMarket(contracts.MarketNormalizedEvent{Symbol: "BTCUSDT", LastPX: 123})
			}
		}
	}()
	go func() {
		defer wg.Done()
		ws := 10
		for {
			select {
			case <-done:
				return
			default:
				raw, _ := json.Marshal(paramReloadPayload{
					Symbol: "BTCUSDT", WindowSize: ws, OrderQty: 1, BreakoutThreshold: 0.001,
				})
				_ = s.ApplyParams(raw)
				ws++
				if ws > 30 {
					ws = 10
				}
			}
		}
	}()
	wg.Wait()
	// If we got here without -race complaining, the mutex discipline is correct.
}
