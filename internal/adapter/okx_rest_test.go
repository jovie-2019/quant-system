package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOKXSpotTradeGatewayPlaceAndCancel(t *testing.T) {
	var placeSeen bool
	var cancelSeen bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("OK-ACCESS-KEY") != "okx-key" {
			t.Fatalf("missing okx api key")
		}
		if r.Header.Get("OK-ACCESS-SIGN") == "" {
			t.Fatalf("missing okx signature")
		}
		if r.Header.Get("OK-ACCESS-TIMESTAMP") == "" {
			t.Fatalf("missing okx timestamp")
		}
		if r.Header.Get("OK-ACCESS-PASSPHRASE") != "okx-pass" {
			t.Fatalf("missing okx passphrase")
		}
		if got := r.Header.Get("x-simulated-trading"); got != "" {
			t.Fatalf("unexpected simulated trading header in default mode: %s", got)
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body error: %v", err)
		}

		switch r.URL.Path {
		case "/api/v5/trade/order":
			placeSeen = true
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for place: %s", r.Method)
			}
			var req map[string]string
			if err := json.Unmarshal(bodyBytes, &req); err != nil {
				t.Fatalf("decode place body error: %v", err)
			}
			if req["instId"] != "BTC-USDT" {
				t.Fatalf("unexpected instId: %s", req["instId"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "0",
				"msg":  "",
				"data": []map[string]string{
					{"ordId": "778899", "clOrdId": "cid-2", "sCode": "0", "sMsg": ""},
				},
			})
		case "/api/v5/trade/cancel-order":
			cancelSeen = true
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for cancel: %s", r.Method)
			}
			var req map[string]string
			if err := json.Unmarshal(bodyBytes, &req); err != nil {
				t.Fatalf("decode cancel body error: %v", err)
			}
			if req["clOrdId"] != "cid-2" {
				t.Fatalf("unexpected clOrdId: %s", req["clOrdId"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "0",
				"msg":  "",
				"data": []map[string]string{
					{"ordId": "778899", "clOrdId": "cid-2", "sCode": "0", "sMsg": ""},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	gw, err := NewOKXSpotTradeGateway(OKXSpotRESTConfig{
		BaseURL:    srv.URL,
		APIKey:     "okx-key",
		APISecret:  "okx-secret",
		Passphrase: "okx-pass",
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewOKXSpotTradeGateway() error = %v", err)
	}
	gw.now = func() time.Time { return time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC) }

	ack, err := gw.PlaceOrder(context.Background(), VenueOrderRequest{
		ClientOrderID: "cid-2",
		Symbol:        "BTC-USDT",
		Side:          "buy",
		Price:         62010,
		Quantity:      0.3,
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	if ack.ClientOrderID != "cid-2" || ack.VenueOrderID != "778899" {
		t.Fatalf("unexpected place ack: %+v", ack)
	}

	cancelAck, err := gw.CancelOrder(context.Background(), VenueCancelRequest{
		ClientOrderID: "cid-2",
		Symbol:        "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if cancelAck.ClientOrderID != "cid-2" || cancelAck.VenueOrderID != "778899" || cancelAck.Status != "canceled" {
		t.Fatalf("unexpected cancel ack: %+v", cancelAck)
	}

	if !placeSeen || !cancelSeen {
		t.Fatalf("expected both place and cancel to be called, place=%v cancel=%v", placeSeen, cancelSeen)
	}
}

func TestOKXSpotTradeGatewaySimulatedHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-simulated-trading"); got != "1" {
			t.Fatalf("expected x-simulated-trading=1, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "0",
			"msg":  "",
			"data": []map[string]string{
				{"ordId": "1", "clOrdId": "cid-sim", "sCode": "0", "sMsg": ""},
			},
		})
	}))
	defer srv.Close()

	gw, err := NewOKXSpotTradeGateway(OKXSpotRESTConfig{
		BaseURL:          srv.URL,
		APIKey:           "okx-key",
		APISecret:        "okx-secret",
		Passphrase:       "okx-pass",
		SimulatedTrading: true,
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewOKXSpotTradeGateway() error = %v", err)
	}
	gw.now = func() time.Time { return time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC) }

	_, err = gw.PlaceOrder(context.Background(), VenueOrderRequest{
		ClientOrderID: "cid-sim",
		Symbol:        "BTC-USDT",
		Side:          "buy",
		Price:         1,
		Quantity:      1,
	})
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
}

func TestNewOKXSpotTradeGatewayRejectsInvalidConfig(t *testing.T) {
	_, err := NewOKXSpotTradeGateway(OKXSpotRESTConfig{
		BaseURL: "",
	}, nil)
	if err == nil {
		t.Fatal("expected config validation error")
	}
	if !strings.Contains(err.Error(), "invalid config") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOKXSpotTradeGatewayRequestValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("gateway should reject request before hitting network")
	}))
	defer srv.Close()

	gw, err := NewOKXSpotTradeGateway(OKXSpotRESTConfig{
		BaseURL:    srv.URL,
		APIKey:     "okx-key",
		APISecret:  "okx-secret",
		Passphrase: "okx-pass",
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewOKXSpotTradeGateway() error = %v", err)
	}

	_, err = gw.PlaceOrder(context.Background(), VenueOrderRequest{
		ClientOrderID: "cid-bad",
		Symbol:        "",
		Side:          "buy",
		Price:         100,
		Quantity:      1,
	})
	if !errors.Is(err, ErrGatewayInvalidOrderRequest) {
		t.Fatalf("expected ErrGatewayInvalidOrderRequest, got %v", err)
	}

	_, err = gw.CancelOrder(context.Background(), VenueCancelRequest{
		ClientOrderID: "",
		VenueOrderID:  "",
		Symbol:        "BTC-USDT",
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

func TestOKXSpotTradeGatewayBusinessErrorIsNonRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "51000",
			"msg":  "parameter error",
			"data": []map[string]string{},
		})
	}))
	defer srv.Close()

	gw, err := NewOKXSpotTradeGateway(OKXSpotRESTConfig{
		BaseURL:    srv.URL,
		APIKey:     "okx-key",
		APISecret:  "okx-secret",
		Passphrase: "okx-pass",
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewOKXSpotTradeGateway() error = %v", err)
	}

	_, err = gw.PlaceOrder(context.Background(), VenueOrderRequest{
		ClientOrderID: "cid-err",
		Symbol:        "BTC-USDT",
		Side:          "buy",
		Price:         1,
		Quantity:      1,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrGatewayNonRetryable) {
		t.Fatalf("expected ErrGatewayNonRetryable, got %v", err)
	}
}

func TestOKXSpotTradeGatewayQueryOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/v5/trade/order" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if got := q.Get("clOrdId"); got != "cid-q-2" {
			t.Fatalf("expected clOrdId=cid-q-2, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "0",
			"msg":  "",
			"data": []map[string]string{
				{
					"ordId":     "778899",
					"clOrdId":   "cid-q-2",
					"instId":    "BTC-USDT",
					"state":     "partially_filled",
					"accFillSz": "0.3",
					"avgPx":     "62010",
				},
			},
		})
	}))
	defer srv.Close()

	gw, err := NewOKXSpotTradeGateway(OKXSpotRESTConfig{
		BaseURL:    srv.URL,
		APIKey:     "okx-key",
		APISecret:  "okx-secret",
		Passphrase: "okx-pass",
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewOKXSpotTradeGateway() error = %v", err)
	}
	gw.now = func() time.Time { return time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC) }

	status, err := gw.QueryOrder(context.Background(), VenueOrderQueryRequest{
		ClientOrderID: "cid-q-2",
		Symbol:        "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("QueryOrder() error = %v", err)
	}
	if status.ClientOrderID != "cid-q-2" || status.VenueOrderID != "778899" {
		t.Fatalf("unexpected order identity: %+v", status)
	}
	if status.Symbol != "BTC-USDT" || status.Status != "partially_filled" {
		t.Fatalf("unexpected order status fields: %+v", status)
	}
	if status.FilledQty != 0.3 || status.AvgPrice != 62010 {
		t.Fatalf("unexpected fill fields: %+v", status)
	}
}
