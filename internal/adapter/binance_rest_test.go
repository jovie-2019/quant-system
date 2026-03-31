package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBinanceSpotTradeGatewayPlaceAndCancel(t *testing.T) {
	var placeSeen bool
	var cancelSeen bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MBX-APIKEY") != "test-key" {
			t.Fatalf("missing api key header")
		}

		q := r.URL.Query()
		if q.Get("signature") == "" {
			t.Fatalf("missing signature")
		}
		if q.Get("timestamp") == "" {
			t.Fatalf("missing timestamp")
		}

		switch r.Method {
		case http.MethodPost:
			placeSeen = true
			if r.URL.Path != "/api/v3/order" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if q.Get("symbol") != "BTCUSDT" {
				t.Fatalf("unexpected symbol: %s", q.Get("symbol"))
			}
			if q.Get("newClientOrderId") != "cid-1" {
				t.Fatalf("unexpected client order id: %s", q.Get("newClientOrderId"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orderId":       12345,
				"clientOrderId": "cid-1",
				"status":        "NEW",
			})
		case http.MethodDelete:
			cancelSeen = true
			if r.URL.Path != "/api/v3/order" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if q.Get("origClientOrderId") != "cid-1" {
				t.Fatalf("unexpected orig client order id: %s", q.Get("origClientOrderId"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orderId":       12345,
				"clientOrderId": "cid-1",
				"status":        "CANCELED",
			})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer srv.Close()

	gw, err := NewBinanceSpotTradeGateway(BinanceSpotRESTConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		APISecret:    "test-secret",
		RecvWindowMS: 5000,
		MinInterval:  0,
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewBinanceSpotTradeGateway() error = %v", err)
	}
	gw.now = func() time.Time { return time.UnixMilli(1700000000000) }

	ack, err := gw.PlaceOrder(context.Background(), VenueOrderRequest{
		ClientOrderID: "cid-1",
		Symbol:        "BTC-USDT",
		Side:          "buy",
		Price:         62000.1,
		Quantity:      0.2,
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if ack.ClientOrderID != "cid-1" || ack.VenueOrderID != "12345" || ack.Status != "new" {
		t.Fatalf("unexpected place ack: %+v", ack)
	}

	cancelAck, err := gw.CancelOrder(context.Background(), VenueCancelRequest{
		ClientOrderID: "cid-1",
		Symbol:        "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if cancelAck.ClientOrderID != "cid-1" || cancelAck.Status != "canceled" {
		t.Fatalf("unexpected cancel ack: %+v", cancelAck)
	}

	if !placeSeen || !cancelSeen {
		t.Fatalf("expected both place and cancel to be called, place=%v cancel=%v", placeSeen, cancelSeen)
	}
}

func TestNewBinanceSpotTradeGatewayRejectsInvalidConfig(t *testing.T) {
	_, err := NewBinanceSpotTradeGateway(BinanceSpotRESTConfig{
		BaseURL: "",
	}, nil)
	if err == nil {
		t.Fatal("expected config validation error")
	}
	if !strings.Contains(err.Error(), "invalid config") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBinanceSpotTradeGatewayRequestValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("gateway should reject request before hitting network")
	}))
	defer srv.Close()

	gw, err := NewBinanceSpotTradeGateway(BinanceSpotRESTConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		APISecret:    "test-secret",
		RecvWindowMS: 5000,
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewBinanceSpotTradeGateway() error = %v", err)
	}

	_, err = gw.PlaceOrder(context.Background(), VenueOrderRequest{
		ClientOrderID: "",
		Symbol:        "BTC-USDT",
		Side:          "buy",
		Price:         100,
		Quantity:      0.1,
	})
	if !errors.Is(err, ErrGatewayInvalidOrderRequest) {
		t.Fatalf("expected ErrGatewayInvalidOrderRequest, got %v", err)
	}

	_, err = gw.CancelOrder(context.Background(), VenueCancelRequest{
		Symbol: "BTC-USDT",
	})
	if !errors.Is(err, ErrGatewayInvalidCancelRequest) {
		t.Fatalf("expected ErrGatewayInvalidCancelRequest, got %v", err)
	}

	_, err = gw.QueryOrder(context.Background(), VenueOrderQueryRequest{
		Symbol: "BTC-USDT",
	})
	if !errors.Is(err, ErrGatewayInvalidQueryRequest) {
		t.Fatalf("expected ErrGatewayInvalidQueryRequest, got %v", err)
	}
}

func TestBinanceSpotTradeGatewayHTTPErrorClassification(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		expect     error
	}{
		{name: "retryable_429", statusCode: http.StatusTooManyRequests, expect: ErrGatewayRetryable},
		{name: "non_retryable_400", statusCode: http.StatusBadRequest, expect: ErrGatewayNonRetryable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(`{"code":-1,"msg":"error"}`))
			}))
			defer srv.Close()

			gw, err := NewBinanceSpotTradeGateway(BinanceSpotRESTConfig{
				BaseURL:      srv.URL,
				APIKey:       "test-key",
				APISecret:    "test-secret",
				RecvWindowMS: 5000,
			}, srv.Client())
			if err != nil {
				t.Fatalf("NewBinanceSpotTradeGateway() error = %v", err)
			}
			gw.now = func() time.Time { return time.UnixMilli(1700000000000) }

			_, err = gw.PlaceOrder(context.Background(), VenueOrderRequest{
				ClientOrderID: "cid-err",
				Symbol:        "BTC-USDT",
				Side:          "buy",
				Price:         100,
				Quantity:      0.1,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tc.expect) {
				t.Fatalf("expected %v, got %v", tc.expect, err)
			}
		})
	}
}

func TestBinanceSpotTradeGatewayCancelByVenueOrderID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		q := r.URL.Query()
		if got := q.Get("orderId"); got != "998877" {
			t.Fatalf("expected orderId=998877, got %q", got)
		}
		if got := q.Get("origClientOrderId"); got != "" {
			t.Fatalf("expected empty origClientOrderId, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"orderId": 998877,
			"status":  "CANCELED",
		})
	}))
	defer srv.Close()

	gw, err := NewBinanceSpotTradeGateway(BinanceSpotRESTConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		APISecret:    "test-secret",
		RecvWindowMS: 5000,
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewBinanceSpotTradeGateway() error = %v", err)
	}
	gw.now = func() time.Time { return time.UnixMilli(1700000000000) }

	ack, err := gw.CancelOrder(context.Background(), VenueCancelRequest{
		VenueOrderID: "998877",
		Symbol:       "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if ack.VenueOrderID != "998877" || ack.Status != "canceled" {
		t.Fatalf("unexpected cancel ack: %+v", ack)
	}
}

func TestBinanceSpotTradeGatewayQueryOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		q := r.URL.Query()
		if got := q.Get("origClientOrderId"); got != "cid-q-1" {
			t.Fatalf("expected origClientOrderId=cid-q-1, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"symbol":              "BTCUSDT",
			"orderId":             112233,
			"clientOrderId":       "cid-q-1",
			"status":              "PARTIALLY_FILLED",
			"executedQty":         "0.20000000",
			"cummulativeQuoteQty": "12400.00000000",
			"price":               "62000.00000000",
		})
	}))
	defer srv.Close()

	gw, err := NewBinanceSpotTradeGateway(BinanceSpotRESTConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		APISecret:    "test-secret",
		RecvWindowMS: 5000,
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewBinanceSpotTradeGateway() error = %v", err)
	}
	gw.now = func() time.Time { return time.UnixMilli(1700000000000) }

	status, err := gw.QueryOrder(context.Background(), VenueOrderQueryRequest{
		ClientOrderID: "cid-q-1",
		Symbol:        "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("QueryOrder() error = %v", err)
	}
	if status.ClientOrderID != "cid-q-1" || status.VenueOrderID != "112233" {
		t.Fatalf("unexpected order identity: %+v", status)
	}
	if status.Symbol != "BTC-USDT" || status.Status != "partially_filled" {
		t.Fatalf("unexpected order status fields: %+v", status)
	}
	if status.FilledQty <= 0 || status.AvgPrice <= 0 {
		t.Fatalf("expected filled qty and avg price > 0, got %+v", status)
	}
}
