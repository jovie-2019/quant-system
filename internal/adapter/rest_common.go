package adapter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

var (
	ErrGatewayInvalidOrderRequest  = errors.New("adapter/gateway: invalid order request")
	ErrGatewayInvalidCancelRequest = errors.New("adapter/gateway: invalid cancel request")
	ErrGatewayInvalidQueryRequest  = errors.New("adapter/gateway: invalid query request")
	ErrGatewayRetryable            = errors.New("adapter/gateway: retryable")
	ErrGatewayNonRetryable         = errors.New("adapter/gateway: non-retryable")
)

type requestPacer struct {
	mu          sync.Mutex
	minInterval time.Duration
	last        time.Time
}

func newRequestPacer(minInterval time.Duration) *requestPacer {
	return &requestPacer{minInterval: minInterval}
}

func (p *requestPacer) wait(ctx context.Context, now func() time.Time) error {
	if p.minInterval <= 0 {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	current := now()
	elapsed := current.Sub(p.last)
	if elapsed >= p.minInterval {
		p.last = current
		return nil
	}

	waitFor := p.minInterval - elapsed
	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		p.last = now()
		return nil
	}
}

func normalizeCanonicalSymbol(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if strings.Contains(s, "-") {
		return s
	}
	switch {
	case strings.HasSuffix(s, "USDT"):
		return s[:len(s)-4] + "-USDT"
	case strings.HasSuffix(s, "USDC"):
		return s[:len(s)-4] + "-USDC"
	case strings.HasSuffix(s, "BTC"):
		return s[:len(s)-3] + "-BTC"
	case strings.HasSuffix(s, "ETH"):
		return s[:len(s)-3] + "-ETH"
	default:
		return s
	}
}

// newHTTPClient creates an http.Client with SOCKS5 proxy support if ALL_PROXY is set.
func newHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	for _, env := range []string{"ALL_PROXY", "all_proxy"} {
		val := os.Getenv(env)
		if strings.HasPrefix(val, "socks5://") {
			addr := strings.TrimPrefix(val, "socks5://")
			dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
			if err != nil {
				slog.Warn("rest: socks5 proxy init failed", "addr", addr, "error", err)
				break
			}
			transport.DialContext = func(ctx context.Context, network, a string) (net.Conn, error) {
				return dialer.Dial(network, a)
			}
			slog.Info("rest: using SOCKS5 proxy", "addr", addr)
			break
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

func canonicalToBinanceSymbol(symbol string) string {
	return strings.ReplaceAll(normalizeCanonicalSymbol(symbol), "-", "")
}

func canonicalToOKXInstID(symbol string) string {
	return normalizeCanonicalSymbol(symbol)
}

func formatDecimal(value float64) string {
	formatted := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.12f", value), "0"), ".")
	if formatted == "" {
		return "0"
	}
	return formatted
}

func validateOrderRequest(req VenueOrderRequest) error {
	if strings.TrimSpace(req.ClientOrderID) == "" {
		return fmt.Errorf("%w: client_order_id is empty", ErrGatewayInvalidOrderRequest)
	}
	if strings.TrimSpace(req.Symbol) == "" {
		return fmt.Errorf("%w: symbol is empty", ErrGatewayInvalidOrderRequest)
	}
	side := strings.ToLower(strings.TrimSpace(req.Side))
	if side != "buy" && side != "sell" {
		return fmt.Errorf("%w: side must be buy/sell", ErrGatewayInvalidOrderRequest)
	}
	if req.Price <= 0 {
		return fmt.Errorf("%w: price must be > 0", ErrGatewayInvalidOrderRequest)
	}
	if req.Quantity <= 0 {
		return fmt.Errorf("%w: quantity must be > 0", ErrGatewayInvalidOrderRequest)
	}
	return nil
}

func validateCancelRequest(req VenueCancelRequest) error {
	if strings.TrimSpace(req.Symbol) == "" {
		return fmt.Errorf("%w: symbol is empty", ErrGatewayInvalidCancelRequest)
	}
	if strings.TrimSpace(req.ClientOrderID) == "" && strings.TrimSpace(req.VenueOrderID) == "" {
		return fmt.Errorf("%w: client_order_id and venue_order_id are both empty", ErrGatewayInvalidCancelRequest)
	}
	return nil
}

func validateQueryRequest(req VenueOrderQueryRequest) error {
	if strings.TrimSpace(req.Symbol) == "" {
		return fmt.Errorf("%w: symbol is empty", ErrGatewayInvalidQueryRequest)
	}
	if strings.TrimSpace(req.ClientOrderID) == "" && strings.TrimSpace(req.VenueOrderID) == "" {
		return fmt.Errorf("%w: client_order_id and venue_order_id are both empty", ErrGatewayInvalidQueryRequest)
	}
	return nil
}

func classifyTransportError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrGatewayRetryable, err)
}

func classifyHTTPError(base error, statusCode int, body string) error {
	err := fmt.Errorf("%w: status=%d body=%s", base, statusCode, body)
	if statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: %v", ErrGatewayRetryable, err)
	}
	if statusCode >= http.StatusBadRequest {
		return fmt.Errorf("%w: %v", ErrGatewayNonRetryable, err)
	}
	return err
}

func asNonRetryable(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrGatewayNonRetryable, err)
}

func parseDecimal(value string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return v
}
