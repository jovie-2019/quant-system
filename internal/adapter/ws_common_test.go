package adapter

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestWithDefaults(t *testing.T) {
	t.Run("zero values get defaults", func(t *testing.T) {
		cfg := withDefaults(wsRuntimeConfig{})
		if cfg.ReconnectMin != 300*time.Millisecond {
			t.Errorf("ReconnectMin = %v, want 300ms", cfg.ReconnectMin)
		}
		if cfg.ReconnectMax != 3*time.Second {
			t.Errorf("ReconnectMax = %v, want 3s", cfg.ReconnectMax)
		}
		if cfg.PingInterval != 15*time.Second {
			t.Errorf("PingInterval = %v, want 15s", cfg.PingInterval)
		}
		if cfg.ReadTimeout != 45*time.Second {
			t.Errorf("ReadTimeout = %v, want 45s", cfg.ReadTimeout)
		}
		if cfg.WriteTimeout != 3*time.Second {
			t.Errorf("WriteTimeout = %v, want 3s", cfg.WriteTimeout)
		}
	})

	t.Run("custom values preserved", func(t *testing.T) {
		cfg := withDefaults(wsRuntimeConfig{
			ReconnectMin: 1 * time.Second,
			ReconnectMax: 10 * time.Second,
			PingInterval: 30 * time.Second,
			ReadTimeout:  60 * time.Second,
			WriteTimeout: 5 * time.Second,
		})
		if cfg.ReconnectMin != 1*time.Second {
			t.Errorf("ReconnectMin = %v, want 1s", cfg.ReconnectMin)
		}
		if cfg.ReconnectMax != 10*time.Second {
			t.Errorf("ReconnectMax = %v, want 10s", cfg.ReconnectMax)
		}
		if cfg.PingInterval != 30*time.Second {
			t.Errorf("PingInterval = %v, want 30s", cfg.PingInterval)
		}
		if cfg.ReadTimeout != 60*time.Second {
			t.Errorf("ReadTimeout = %v, want 60s", cfg.ReadTimeout)
		}
		if cfg.WriteTimeout != 5*time.Second {
			t.Errorf("WriteTimeout = %v, want 5s", cfg.WriteTimeout)
		}
	})

	t.Run("reconnect max clamped to min when smaller", func(t *testing.T) {
		cfg := withDefaults(wsRuntimeConfig{
			ReconnectMin: 5 * time.Second,
			ReconnectMax: 1 * time.Second,
		})
		if cfg.ReconnectMax != cfg.ReconnectMin {
			t.Errorf("ReconnectMax = %v, want %v (clamped to min)", cfg.ReconnectMax, cfg.ReconnectMin)
		}
	})

	t.Run("negative values treated as zero", func(t *testing.T) {
		cfg := withDefaults(wsRuntimeConfig{
			ReconnectMin: -1,
			PingInterval: -1,
		})
		if cfg.ReconnectMin != 300*time.Millisecond {
			t.Errorf("ReconnectMin = %v, want 300ms", cfg.ReconnectMin)
		}
		if cfg.PingInterval != 15*time.Second {
			t.Errorf("PingInterval = %v, want 15s", cfg.PingInterval)
		}
	})
}

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name    string
		current time.Duration
		max     time.Duration
		want    time.Duration
	}{
		{"doubles", 100 * time.Millisecond, 10 * time.Second, 200 * time.Millisecond},
		{"doubles again", 500 * time.Millisecond, 10 * time.Second, 1 * time.Second},
		{"clamps to max", 3 * time.Second, 5 * time.Second, 5 * time.Second},
		{"already at max", 5 * time.Second, 5 * time.Second, 5 * time.Second},
		{"exceeds max", 4 * time.Second, 5 * time.Second, 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextBackoff(tt.current, tt.max)
			if got != tt.want {
				t.Errorf("nextBackoff(%v, %v) = %v, want %v", tt.current, tt.max, got, tt.want)
			}
		})
	}
}

func TestToCanonicalFromCompactSymbol(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"btcusdt", "BTC-USDT"},
		{"BTC-USDT", "BTC-USDT"},
		{"ethusdc", "ETH-USDC"},
		{"solbtc", "SOL-BTC"},
		{"linketh", "LINK-ETH"},
		{" btcusdt ", "BTC-USDT"},
		{"XRP", "XRP"},
		{"ETHUSDT", "ETH-USDT"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toCanonicalFromCompactSymbol(tt.input)
			if got != tt.want {
				t.Errorf("toCanonicalFromCompactSymbol(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSleepOrDone(t *testing.T) {
	t.Run("returns true when timer fires", func(t *testing.T) {
		ok := sleepOrDone(context.Background(), 1*time.Millisecond)
		if !ok {
			t.Error("expected true")
		}
	})

	t.Run("returns false when context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		ok := sleepOrDone(ctx, 10*time.Second)
		if ok {
			t.Error("expected false")
		}
	})
}

func TestBuildNormalizedPayload(t *testing.T) {
	payload := buildNormalizedPayload("100.5", "1.2", "101.0", "0.8", "100.7", 42, 1700000000000)

	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	checks := map[string]string{
		"bid_px":  "100.5",
		"bid_sz":  "1.2",
		"ask_px":  "101.0",
		"ask_sz":  "0.8",
		"last_px": "100.7",
		"seq":     "42",
		"ts":      "1700000000000",
	}
	for k, want := range checks {
		got, ok := m[k].(string)
		if !ok {
			t.Errorf("key %q not a string", k)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	t.Run("empty last_px falls back to bid_px", func(t *testing.T) {
		payload := buildNormalizedPayload("99.0", "1", "100.0", "1", "", 1, 1)
		var m map[string]any
		if err := json.Unmarshal(payload, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if got := m["last_px"].(string); got != "99.0" {
			t.Errorf("last_px = %q, want %q", got, "99.0")
		}
	})
}
