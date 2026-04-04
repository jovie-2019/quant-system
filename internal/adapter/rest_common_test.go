package adapter

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestNormalizeCanonicalSymbol(t *testing.T) {
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
		{"eth-usdc", "ETH-USDC"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeCanonicalSymbol(tt.input)
			if got != tt.want {
				t.Errorf("normalizeCanonicalSymbol(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCanonicalToBinanceSymbol(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"BTC-USDT", "BTCUSDT"},
		{"ETH-USDC", "ETHUSDC"},
		{"btcusdt", "BTCUSDT"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := canonicalToBinanceSymbol(tt.input)
			if got != tt.want {
				t.Errorf("canonicalToBinanceSymbol(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCanonicalToOKXInstID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"BTC-USDT", "BTC-USDT"},
		{"btcusdt", "BTC-USDT"},
		{"ETH-USDC", "ETH-USDC"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := canonicalToOKXInstID(tt.input)
			if got != tt.want {
				t.Errorf("canonicalToOKXInstID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatDecimal(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0.1, "0.1"},
		{62000.0, "62000"},
		{0.00001, "0.00001"},
		{1.123456789012, "1.123456789012"},
		{0.0, "0"},
		{100.50, "100.5"},
		{0.000000000001, "0.000000000001"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDecimal(tt.input)
			if got != tt.want {
				t.Errorf("formatDecimal(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDecimal(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1.5", 1.5},
		{" 100 ", 100.0},
		{"invalid", 0},
		{"", 0},
		{"0.00001", 0.00001},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseDecimal(tt.input)
			if got != tt.want {
				t.Errorf("parseDecimal(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRequestPacerWait(t *testing.T) {
	t.Run("first call returns immediately", func(t *testing.T) {
		p := newRequestPacer(100 * time.Millisecond)
		now := time.Now()
		err := p.wait(context.Background(), func() time.Time { return now })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("second call within interval waits", func(t *testing.T) {
		p := newRequestPacer(50 * time.Millisecond)
		now := time.Now()
		callCount := 0
		clock := func() time.Time {
			callCount++
			if callCount == 1 {
				return now
			}
			// Second call is 10ms later, within the 50ms interval.
			return now.Add(10 * time.Millisecond)
		}

		// First call sets the baseline.
		if err := p.wait(context.Background(), clock); err != nil {
			t.Fatalf("first wait: %v", err)
		}

		// Second call should block until the interval elapses.
		start := time.Now()
		if err := p.wait(context.Background(), clock); err != nil {
			t.Fatalf("second wait: %v", err)
		}
		elapsed := time.Since(start)
		if elapsed < 30*time.Millisecond {
			t.Errorf("expected wait of ~40ms, got %v", elapsed)
		}
	})

	t.Run("context cancellation returns error", func(t *testing.T) {
		p := newRequestPacer(1 * time.Second)
		now := time.Now()
		callCount := 0
		clock := func() time.Time {
			callCount++
			if callCount == 1 {
				return now
			}
			return now.Add(1 * time.Millisecond)
		}

		// First call to set baseline.
		_ = p.wait(context.Background(), clock)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := p.wait(ctx, clock)
		if err == nil {
			t.Fatal("expected error from cancelled context")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("zero interval returns immediately", func(t *testing.T) {
		p := newRequestPacer(0)
		err := p.wait(context.Background(), time.Now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateOrderRequest(t *testing.T) {
	valid := VenueOrderRequest{
		ClientOrderID: "cid-1",
		Symbol:        "BTC-USDT",
		Side:          "buy",
		Price:         100.0,
		Quantity:      1.0,
	}

	t.Run("valid request", func(t *testing.T) {
		if err := validateOrderRequest(valid); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty client_order_id", func(t *testing.T) {
		req := valid
		req.ClientOrderID = ""
		err := validateOrderRequest(req)
		if !errors.Is(err, ErrGatewayInvalidOrderRequest) {
			t.Errorf("expected ErrGatewayInvalidOrderRequest, got %v", err)
		}
	})

	t.Run("empty symbol", func(t *testing.T) {
		req := valid
		req.Symbol = ""
		err := validateOrderRequest(req)
		if !errors.Is(err, ErrGatewayInvalidOrderRequest) {
			t.Errorf("expected ErrGatewayInvalidOrderRequest, got %v", err)
		}
	})

	t.Run("invalid side", func(t *testing.T) {
		req := valid
		req.Side = "hold"
		err := validateOrderRequest(req)
		if !errors.Is(err, ErrGatewayInvalidOrderRequest) {
			t.Errorf("expected ErrGatewayInvalidOrderRequest, got %v", err)
		}
	})

	t.Run("zero price", func(t *testing.T) {
		req := valid
		req.Price = 0
		err := validateOrderRequest(req)
		if !errors.Is(err, ErrGatewayInvalidOrderRequest) {
			t.Errorf("expected ErrGatewayInvalidOrderRequest, got %v", err)
		}
	})

	t.Run("negative quantity", func(t *testing.T) {
		req := valid
		req.Quantity = -1
		err := validateOrderRequest(req)
		if !errors.Is(err, ErrGatewayInvalidOrderRequest) {
			t.Errorf("expected ErrGatewayInvalidOrderRequest, got %v", err)
		}
	})

	t.Run("sell side accepted", func(t *testing.T) {
		req := valid
		req.Side = "sell"
		if err := validateOrderRequest(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestValidateCancelRequest(t *testing.T) {
	t.Run("valid with client_order_id", func(t *testing.T) {
		req := VenueCancelRequest{Symbol: "BTC-USDT", ClientOrderID: "cid-1"}
		if err := validateCancelRequest(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid with venue_order_id", func(t *testing.T) {
		req := VenueCancelRequest{Symbol: "BTC-USDT", VenueOrderID: "vid-1"}
		if err := validateCancelRequest(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty symbol", func(t *testing.T) {
		req := VenueCancelRequest{ClientOrderID: "cid-1"}
		err := validateCancelRequest(req)
		if !errors.Is(err, ErrGatewayInvalidCancelRequest) {
			t.Errorf("expected ErrGatewayInvalidCancelRequest, got %v", err)
		}
	})

	t.Run("both ids empty", func(t *testing.T) {
		req := VenueCancelRequest{Symbol: "BTC-USDT"}
		err := validateCancelRequest(req)
		if !errors.Is(err, ErrGatewayInvalidCancelRequest) {
			t.Errorf("expected ErrGatewayInvalidCancelRequest, got %v", err)
		}
	})
}

func TestValidateQueryRequest(t *testing.T) {
	t.Run("valid with client_order_id", func(t *testing.T) {
		req := VenueOrderQueryRequest{Symbol: "BTC-USDT", ClientOrderID: "cid-1"}
		if err := validateQueryRequest(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty symbol", func(t *testing.T) {
		req := VenueOrderQueryRequest{ClientOrderID: "cid-1"}
		err := validateQueryRequest(req)
		if !errors.Is(err, ErrGatewayInvalidQueryRequest) {
			t.Errorf("expected ErrGatewayInvalidQueryRequest, got %v", err)
		}
	})

	t.Run("both ids empty", func(t *testing.T) {
		req := VenueOrderQueryRequest{Symbol: "BTC-USDT"}
		err := validateQueryRequest(req)
		if !errors.Is(err, ErrGatewayInvalidQueryRequest) {
			t.Errorf("expected ErrGatewayInvalidQueryRequest, got %v", err)
		}
	})
}

func TestClassifyTransportError(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		if err := classifyTransportError(nil); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("context canceled passes through", func(t *testing.T) {
		err := classifyTransportError(context.Canceled)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("deadline exceeded passes through", func(t *testing.T) {
		err := classifyTransportError(context.DeadlineExceeded)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got %v", err)
		}
	})

	t.Run("other errors wrapped as retryable", func(t *testing.T) {
		err := classifyTransportError(errors.New("connection reset"))
		if !errors.Is(err, ErrGatewayRetryable) {
			t.Errorf("expected ErrGatewayRetryable, got %v", err)
		}
	})
}

func TestClassifyHTTPError(t *testing.T) {
	base := errors.New("test")

	t.Run("429 is retryable", func(t *testing.T) {
		err := classifyHTTPError(base, http.StatusTooManyRequests, "rate limited")
		if !errors.Is(err, ErrGatewayRetryable) {
			t.Errorf("expected ErrGatewayRetryable, got %v", err)
		}
	})

	t.Run("500 is retryable", func(t *testing.T) {
		err := classifyHTTPError(base, http.StatusInternalServerError, "oops")
		if !errors.Is(err, ErrGatewayRetryable) {
			t.Errorf("expected ErrGatewayRetryable, got %v", err)
		}
	})

	t.Run("400 is non-retryable", func(t *testing.T) {
		err := classifyHTTPError(base, http.StatusBadRequest, "bad")
		if !errors.Is(err, ErrGatewayNonRetryable) {
			t.Errorf("expected ErrGatewayNonRetryable, got %v", err)
		}
	})

	t.Run("200 returns base error info", func(t *testing.T) {
		err := classifyHTTPError(base, 200, "ok")
		if errors.Is(err, ErrGatewayRetryable) || errors.Is(err, ErrGatewayNonRetryable) {
			t.Errorf("expected unwrapped error, got %v", err)
		}
	})
}

func TestAsNonRetryable(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		if err := asNonRetryable(nil); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("wraps error as non-retryable", func(t *testing.T) {
		err := asNonRetryable(errors.New("something"))
		if !errors.Is(err, ErrGatewayNonRetryable) {
			t.Errorf("expected ErrGatewayNonRetryable, got %v", err)
		}
	})
}
