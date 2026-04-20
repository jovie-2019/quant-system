package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"quant-system/internal/marketstore"
	"quant-system/internal/regime"
	"quant-system/pkg/contracts"
)

// newRegimeServer builds a Server with in-memory stores wired in so the
// compute → history → matrix round-trip can run end-to-end without
// spinning up ClickHouse.
func newRegimeServer(t *testing.T, klines []contracts.Kline) *Server {
	t.Helper()
	ks := marketstore.NewMemoryStore()
	if len(klines) > 0 {
		if err := ks.Upsert(context.Background(), klines); err != nil {
			t.Fatalf("seed klines: %v", err)
		}
	}
	return &Server{
		logger:  slog.Default(),
		klines:  ks,
		regimes: marketstore.NewInMemoryRegimeStore(),
	}
}

func genTrendingKlines(n int) []contracts.Kline {
	out := make([]contracts.Kline, n)
	baseTS := int64(1_700_000_000_000)
	for i := 0; i < n; i++ {
		px := 100 * math.Exp(0.0015*float64(i))
		out[i] = contracts.Kline{
			Venue:     contracts.VenueBinance,
			Symbol:    "BTCUSDT",
			Interval:  "1m",
			OpenTime:  baseTS + int64(i)*60_000,
			CloseTime: baseTS + int64(i)*60_000 + 59_999,
			Open:      px, High: px * 1.0005, Low: px * 0.9995, Close: px, Volume: 1,
			Closed: true,
		}
	}
	return out
}

func TestHandleComputeRegime_Success(t *testing.T) {
	klines := genTrendingKlines(400)
	srv := newRegimeServer(t, klines)

	body := map[string]any{
		"venue":    "binance",
		"symbol":   "BTCUSDT",
		"interval": "1m",
		"start_ms": klines[0].OpenTime,
		"end_ms":   klines[len(klines)-1].CloseTime,
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/regime/compute", bytes.NewReader(raw))
	rr := httptest.NewRecorder()

	srv.HandleComputeRegime(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp ComputeRegimeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RecordsStored == 0 {
		t.Fatal("expected records stored")
	}
	if resp.Latest == nil {
		t.Fatal("latest nil")
	}
	if resp.Latest.Regime == "" {
		t.Fatal("latest regime empty")
	}
}

func TestHandleComputeRegime_MissingStoreReturns503(t *testing.T) {
	srv := &Server{logger: slog.Default()} // no klines / no regimes
	req := httptest.NewRequest(http.MethodPost, "/api/v1/regime/compute",
		bytes.NewReader([]byte(`{"symbol":"X","interval":"1m","start_ms":1,"end_ms":2}`)))
	rr := httptest.NewRecorder()
	srv.HandleComputeRegime(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", rr.Code)
	}
}

func TestHandleComputeRegime_BadRequest(t *testing.T) {
	srv := newRegimeServer(t, nil)
	cases := []string{
		`{"interval":"1m","start_ms":1,"end_ms":2}`,       // missing symbol
		`{"symbol":"X","start_ms":1,"end_ms":2}`,          // missing interval
		`{"symbol":"X","interval":"1m","end_ms":2}`,       // missing start
		`{"symbol":"X","interval":"1m","start_ms":2,"end_ms":1}`, // start >= end
	}
	for i, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/regime/compute", bytes.NewReader([]byte(body)))
		rr := httptest.NewRecorder()
		srv.HandleComputeRegime(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("case %d status=%d body=%s", i, rr.Code, rr.Body.String())
		}
	}
}

func TestHandleRegimeHistory_QueriesSeededStore(t *testing.T) {
	srv := newRegimeServer(t, nil)

	// Seed some regime records directly.
	recs := []regime.Record{
		{Venue: "binance", Symbol: "BTCUSDT", Interval: "1m", BarTime: 1000,
			Method: regime.MethodThreshold, Regime: regime.RegimeTrendUp, Confidence: 0.7},
		{Venue: "binance", Symbol: "BTCUSDT", Interval: "1m", BarTime: 2000,
			Method: regime.MethodThreshold, Regime: regime.RegimeRange, Confidence: 0.5},
	}
	_ = srv.regimes.UpsertRegimes(context.Background(), recs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/regime/history?symbol=BTCUSDT&interval=1m", nil)
	rr := httptest.NewRecorder()
	srv.HandleRegimeHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp RegimeHistoryResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Count != 2 {
		t.Fatalf("count=%d want 2", resp.Count)
	}
	if resp.Items[0].BarTime != 1000 || resp.Items[1].BarTime != 2000 {
		t.Fatalf("order wrong: %+v", resp.Items)
	}
}

func TestHandleRegimeMatrix_LatestPerKey(t *testing.T) {
	srv := newRegimeServer(t, nil)

	_ = srv.regimes.UpsertRegimes(context.Background(), []regime.Record{
		{Venue: "binance", Symbol: "BTCUSDT", Interval: "1m", BarTime: 1000,
			Method: regime.MethodThreshold, Regime: regime.RegimeTrendUp, Confidence: 0.7},
		{Venue: "binance", Symbol: "BTCUSDT", Interval: "1m", BarTime: 2000,
			Method: regime.MethodThreshold, Regime: regime.RegimeRange, Confidence: 0.8},
		{Venue: "binance", Symbol: "ETHUSDT", Interval: "5m", BarTime: 3000,
			Method: regime.MethodThreshold, Regime: regime.RegimeHighVol, Confidence: 0.9},
	})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/regime/matrix?venue=binance&symbols=BTCUSDT,ETHUSDT&intervals=1m,5m", nil)
	rr := httptest.NewRecorder()
	srv.HandleRegimeMatrix(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp RegimeMatrixResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	// We expect 2 rows: BTC 1m (latest=range) and ETH 5m (high_vol).
	// The 4-cell grid has 2 cells unseeded (BTC 5m, ETH 1m) → omitted.
	if len(resp.Rows) != 2 {
		t.Fatalf("rows=%d want 2", len(resp.Rows))
	}
	byKey := map[string]RegimeMatrixRow{}
	for _, r := range resp.Rows {
		byKey[r.Symbol+":"+r.Interval] = r
	}
	if byKey["BTCUSDT:1m"].Regime != regime.RegimeRange {
		t.Fatalf("BTC 1m regime=%s want range", byKey["BTCUSDT:1m"].Regime)
	}
	if byKey["ETHUSDT:5m"].Regime != regime.RegimeHighVol {
		t.Fatalf("ETH 5m regime=%s want high_vol", byKey["ETHUSDT:5m"].Regime)
	}
}

func TestHandleRegimeMatrix_RequiresSymbolsAndIntervals(t *testing.T) {
	srv := newRegimeServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/regime/matrix", nil)
	rr := httptest.NewRecorder()
	srv.HandleRegimeMatrix(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}
