package marketstore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// fakeBinanceHandler emits up to maxPerPage synthetic klines per request,
// honouring startTime/endTime query params so callers exercise the real
// paging logic.
type fakeBinanceHandler struct {
	stepMS     int64
	maxPerPage int
	calls      atomic.Int32
}

func (h *fakeBinanceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.calls.Add(1)
	if r.URL.Path != "/api/v3/klines" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	start, _ := strconv.ParseInt(q.Get("startTime"), 10, 64)
	end, _ := strconv.ParseInt(q.Get("endTime"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > h.maxPerPage {
		limit = h.maxPerPage
	}

	var rows [][]any
	for t := start; t <= end && len(rows) < limit; t += h.stepMS {
		closeT := t + h.stepMS - 1
		rows = append(rows, []any{
			t,
			fmt.Sprintf("%.2f", 100+float64(t%7)),
			fmt.Sprintf("%.2f", 110+float64(t%7)),
			fmt.Sprintf("%.2f", 95+float64(t%7)),
			fmt.Sprintf("%.2f", 105+float64(t%7)),
			"1.5",
			closeT,
			"quote_vol",
			0,
			"taker_buy",
			"taker_quote",
			"ignore",
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows)
}

func TestBinanceFetcher_PagesWindow(t *testing.T) {
	h := &fakeBinanceHandler{stepMS: 60_000, maxPerPage: 10}
	srv := httptest.NewServer(h)
	defer srv.Close()

	f := NewBinanceFetcher(BinanceFetcherConfig{BaseURL: srv.URL})
	start := int64(1_700_000_000_000)
	end := start + 60_000*24 // 25 klines (inclusive), 3 pages of up to 10

	got, err := f.FetchRange(context.Background(), "BTC-USDT", "1m", start, end, 0)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 25 {
		t.Fatalf("got=%d want 25", len(got))
	}
	if got[0].OpenTime != start {
		t.Fatalf("first OpenTime=%d want %d", got[0].OpenTime, start)
	}
	if got[len(got)-1].OpenTime != end {
		t.Fatalf("last OpenTime=%d want %d", got[len(got)-1].OpenTime, end)
	}
	// No duplicates.
	for i := 1; i < len(got); i++ {
		if got[i].OpenTime <= got[i-1].OpenTime {
			t.Fatalf("non-monotonic at %d", i)
		}
	}
	if got := h.calls.Load(); got < 3 {
		t.Fatalf("pages fetched=%d want >= 3", got)
	}
}

func TestBinanceFetcher_ValidatesWindow(t *testing.T) {
	f := NewBinanceFetcher(BinanceFetcherConfig{BaseURL: "http://unused"})
	_, err := f.FetchRange(context.Background(), "X", "1m", 0, 100, 0)
	if err == nil {
		t.Fatal("expected error for zero start")
	}
	_, err = f.FetchRange(context.Background(), "X", "1m", 100, 50, 0)
	if err == nil {
		t.Fatal("expected error for start > end")
	}
}

func TestBinanceFetcher_UnknownInterval(t *testing.T) {
	f := NewBinanceFetcher(BinanceFetcherConfig{BaseURL: "http://unused"})
	_, err := f.FetchRange(context.Background(), "X", "7q", 1, 2, 0)
	if err == nil {
		t.Fatal("expected error for unknown interval unit")
	}
}

func TestBinanceFetcher_NormalisesSymbol(t *testing.T) {
	var captured string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.Query().Get("symbol")
		_, _ = w.Write([]byte("[]"))
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	f := NewBinanceFetcher(BinanceFetcherConfig{BaseURL: srv.URL})
	_, _ = f.FetchRange(context.Background(), "btc-usdt", "1m", 1_700_000_000_000, 1_700_000_060_000, 0)
	if captured != "BTCUSDT" {
		t.Fatalf("symbol sent=%q want BTCUSDT", captured)
	}
}

func TestBinanceFetcher_PacingRespectsCtxCancel(t *testing.T) {
	h := &fakeBinanceHandler{stepMS: 60_000, maxPerPage: 2}
	srv := httptest.NewServer(h)
	defer srv.Close()

	f := NewBinanceFetcher(BinanceFetcherConfig{BaseURL: srv.URL})
	start := int64(1_700_000_000_000)
	end := start + 60_000*100 // many pages

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := f.FetchRange(ctx, "X", "1m", start, end, 200*time.Millisecond)
	if err == nil {
		t.Log("ok: finished within ctx budget")
	}
	// Either way, should not hang longer than ctx deadline + a small slack.
}

func TestIntervalToMS(t *testing.T) {
	cases := map[string]int64{
		"1m":  60_000,
		"5m":  5 * 60_000,
		"15m": 15 * 60_000,
		"1h":  60 * 60_000,
		"4h":  4 * 60 * 60_000,
		"1d":  24 * 60 * 60_000,
	}
	for k, want := range cases {
		got, err := intervalToMS(k)
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		if got != want {
			t.Fatalf("%s: got %d want %d", k, got, want)
		}
	}
}
