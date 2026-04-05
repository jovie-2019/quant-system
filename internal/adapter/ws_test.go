package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestParseBinanceBookTicker(t *testing.T) {
	msg := []byte(`{"e":"bookTicker","E":1700000000000,"s":"BTCUSDT","u":101,"b":"62000.1","B":"1.1","a":"62000.2","A":"0.9"}`)
	evt, ok, err := parseBinanceBookTicker(msg)
	if err != nil {
		t.Fatalf("parseBinanceBookTicker() error = %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if evt.Venue != VenueBinance || evt.Symbol != "BTC-USDT" || evt.Sequence != 101 {
		t.Fatalf("unexpected event: %+v", evt)
	}
}

func TestParseOKXTicker(t *testing.T) {
	msg := []byte(`{
		"arg":{"channel":"tickers","instId":"BTC-USDT"},
		"data":[{"last":"62000.3","bidPx":"62000.1","bidSz":"1.1","askPx":"62000.2","askSz":"0.9","ts":"1700000000001"}]
	}`)
	evts, err := parseOKXTicker(msg)
	if err != nil {
		t.Fatalf("parseOKXTicker() error = %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	if evts[0].Venue != VenueOKX || evts[0].Symbol != "BTC-USDT" {
		t.Fatalf("unexpected event: %+v", evts[0])
	}
}

func TestBinanceSpotWSReconnect(t *testing.T) {
	// Clear proxy env vars so tests connect directly to local test server.
	for _, k := range []string{"ALL_PROXY", "all_proxy", "HTTPS_PROXY", "HTTP_PROXY"} {
		t.Setenv(k, "")
	}

	var connCount atomic.Int64

	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer c.Close()

		_, _, _ = c.ReadMessage() // subscribe message
		count := connCount.Add(1)

		msg := `{"e":"bookTicker","E":1700000000000,"s":"BTCUSDT","u":101,"b":"62000.1","B":"1.1","a":"62000.2","A":"0.9"}`
		if count > 1 {
			msg = `{"e":"bookTicker","E":1700000000100,"s":"BTCUSDT","u":102,"b":"62000.3","B":"1.0","a":"62000.4","A":"0.8"}`
		}
		_ = c.WriteMessage(websocket.TextMessage, []byte(msg))

		if count == 1 {
			_ = c.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	stream, err := NewBinanceSpotWSMarketStream(BinanceSpotWSConfig{
		Endpoint:     wsURL,
		ReconnectMin: 20 * time.Millisecond,
		ReconnectMax: 80 * time.Millisecond,
		PingInterval: 30 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewBinanceSpotWSMarketStream() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := stream.Subscribe(ctx, []string{"BTC-USDT"})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	first := <-ch
	second := <-ch
	if first.Sequence != 101 || second.Sequence != 102 {
		t.Fatalf("unexpected sequence values first=%d second=%d", first.Sequence, second.Sequence)
	}
	if stream.ReconnectCount() < 2 {
		t.Fatalf("expected reconnect count >= 2, got %d", stream.ReconnectCount())
	}
}
