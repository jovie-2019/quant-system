package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"quant-system/internal/adapter"
	"quant-system/internal/normalizer"
)

func TestSandboxMarketIngress(t *testing.T) {
	if os.Getenv("RUN_SANDBOX_TESTS") != "1" {
		t.Skip("set RUN_SANDBOX_TESTS=1 to enable sandbox probes")
	}

	symbol := envOrDefault("SANDBOX_SYMBOL", "BTC-USDT")
	timeout := envDuration("SANDBOX_TIMEOUT", 15*time.Second)
	enabledCount := 0

	if envBool("SANDBOX_BINANCE_ENABLED", true) {
		stream, err := adapter.NewBinanceSpotWSMarketStream(adapter.BinanceSpotWSConfig{
			Endpoint: envOrDefault("SANDBOX_BINANCE_WS", "wss://stream.binance.com:9443/ws"),
		})
		if err != nil {
			t.Fatalf("binance stream config error: %v", err)
		}
		enabledCount++
		probeMarketVenue(t, "binance", stream, symbol, timeout)
	}

	if envBool("SANDBOX_OKX_ENABLED", true) {
		stream, err := adapter.NewOKXSpotWSMarketStream(adapter.OKXSpotWSConfig{
			Endpoint: envOrDefault("SANDBOX_OKX_WS", "wss://ws.okx.com:8443/ws/v5/public"),
		})
		if err != nil {
			t.Fatalf("okx stream config error: %v", err)
		}
		enabledCount++
		probeMarketVenue(t, "okx", stream, symbol, timeout)
	}

	if enabledCount == 0 {
		t.Fatal("no sandbox venue is enabled")
	}
}

func probeMarketVenue(t *testing.T, venue string, stream adapter.MarketStream, symbol string, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ch, err := stream.Subscribe(ctx, []string{symbol})
	if err != nil {
		t.Fatalf("%s subscribe error: %v", venue, err)
	}

	n := normalizer.NewJSONNormalizer()
	received := 0
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("%s timeout waiting market event, received=%d", venue, received)
		case evt, ok := <-ch:
			if !ok {
				t.Fatalf("%s stream closed before valid event", venue)
			}
			received++

			normalized, err := n.NormalizeMarket(evt)
			if err != nil {
				continue
			}
			if normalized.Symbol == "" || normalized.SourceTSMS <= 0 {
				continue
			}

			t.Logf("%s probe ok symbol=%s seq=%d ts=%d", venue, normalized.Symbol, normalized.Sequence, normalized.SourceTSMS)
			return
		}
	}
}

func envOrDefault(name, fallback string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	return v
}

func envDuration(name string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func envBool(name string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on", "y":
		return true
	case "0", "false", "no", "off", "n":
		return false
	default:
		return fallback
	}
}
