package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quant-system/internal/marketstore"
	_ "quant-system/internal/strategy/momentum" // registers "momentum" strategy type
	"quant-system/pkg/contracts"
)

// newBacktestServer returns a minimally-wired Server capable of serving the
// backtest endpoints. Auth / DB dependencies are intentionally omitted since
// backtests are pure compute.
func newBacktestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		logger:    slog.Default(),
		backtests: NewBacktestStore(20),
	}
}

func TestHandleCreateBacktest_Success(t *testing.T) {
	srv := newBacktestServer(t)

	body := `{
		"strategy_type": "momentum",
		"strategy_params": {
			"symbol": "BTCUSDT",
			"window_size": 10,
			"breakout_threshold": 0.0005,
			"order_qty": 0.1,
			"time_in_force": "IOC",
			"cooldown_ms": 0
		},
		"dataset": {
			"source": "synthetic",
			"symbol": "BTCUSDT",
			"num_events": 200,
			"seed": 7,
			"start_price": 20000,
			"volatility_bps": 30,
			"trend_bps_per_step": 15
		},
		"start_equity": 10000,
		"slippage_bps": 2,
		"fee_bps": 10,
		"risk": {
			"max_order_qty": 10,
			"max_order_amount": 1000000
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backtests", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.HandleCreateBacktest(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rec BacktestRecord
	if err := json.Unmarshal(rr.Body.Bytes(), &rec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec.ID == "" {
		t.Fatal("empty id")
	}
	if rec.Status != BacktestStatusDone {
		t.Fatalf("status=%s want done (err=%q)", rec.Status, rec.Error)
	}
	if rec.Result == nil {
		t.Fatal("result nil")
	}
	if rec.Result.Events != 200 {
		t.Fatalf("events=%d want 200", rec.Result.Events)
	}
	if rec.Result.StrategyID == "" {
		t.Fatal("strategy id empty")
	}

	// Record should be retrievable via GET.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/backtests/"+rec.ID, nil)
	getRR := httptest.NewRecorder()
	srv.HandleGetBacktest(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRR.Code, getRR.Body.String())
	}
}

func TestHandleCreateBacktest_UnknownStrategyType(t *testing.T) {
	srv := newBacktestServer(t)
	body := `{
		"strategy_type": "does-not-exist",
		"strategy_params": {"symbol": "X"},
		"dataset": {"source": "synthetic", "symbol": "X", "num_events": 10},
		"start_equity": 1000
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backtests", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.HandleCreateBacktest(rr, req)
	if rr.Code != http.StatusCreated {
		// Job is still created as a record, but status=failed.
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rec BacktestRecord
	_ = json.Unmarshal(rr.Body.Bytes(), &rec)
	if rec.Status != BacktestStatusFailed {
		t.Fatalf("status=%s want failed", rec.Status)
	}
	if !strings.Contains(rec.Error, "unknown strategy_type") {
		t.Fatalf("error=%q", rec.Error)
	}
}

func TestHandleCreateBacktest_InvalidBody(t *testing.T) {
	srv := newBacktestServer(t)
	tests := []struct {
		name string
		body string
	}{
		{"missing strategy_type", `{"strategy_params":{},"dataset":{"symbol":"X","num_events":1}}`},
		{"missing strategy_params", `{"strategy_type":"momentum","dataset":{"symbol":"X","num_events":1}}`},
		{"zero events", `{"strategy_type":"momentum","strategy_params":{},"dataset":{"symbol":"X","num_events":0}}`},
		{"too many events", `{"strategy_type":"momentum","strategy_params":{},"dataset":{"symbol":"X","num_events":1000000}}`},
		{"negative slippage", `{"strategy_type":"momentum","strategy_params":{},"dataset":{"symbol":"X","num_events":10},"slippage_bps":-1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/backtests", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()
			srv.HandleCreateBacktest(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleCreateBacktest_WrongMethod(t *testing.T) {
	srv := newBacktestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backtests", nil)
	rr := httptest.NewRecorder()
	srv.HandleCreateBacktest(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleGetBacktest_NotFound(t *testing.T) {
	srv := newBacktestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backtests/bt_missing", nil)
	rr := httptest.NewRecorder()
	srv.HandleGetBacktest(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleListBacktests_ReturnsRecentFirst(t *testing.T) {
	srv := newBacktestServer(t)
	// Create two jobs back to back.
	createOne := func() string {
		body := `{
			"strategy_type": "momentum",
			"strategy_params": {"symbol":"BTCUSDT","window_size":5,"breakout_threshold":0.001,"order_qty":0.01,"cooldown_ms":0},
			"dataset": {"source":"synthetic","symbol":"BTCUSDT","num_events":30,"seed":1,"start_price":100,"volatility_bps":10},
			"start_equity": 1000
		}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backtests", strings.NewReader(body))
		rr := httptest.NewRecorder()
		srv.HandleCreateBacktest(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
		}
		var rec BacktestRecord
		_ = json.Unmarshal(rr.Body.Bytes(), &rec)
		return rec.ID
	}
	firstID := createOne()
	secondID := createOne()

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/backtests?limit=10", nil)
	listRR := httptest.NewRecorder()
	srv.HandleListBacktests(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var resp struct {
		Items []BacktestRecord `json:"items"`
		Count int              `json:"count"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if resp.Count < 2 {
		t.Fatalf("count=%d want >=2", resp.Count)
	}
	// Newest first.
	if resp.Items[0].ID != secondID {
		t.Fatalf("first=%s want %s", resp.Items[0].ID, secondID)
	}
	if resp.Items[1].ID != firstID {
		t.Fatalf("second=%s want %s", resp.Items[1].ID, firstID)
	}
}

func TestHandleCreateBacktest_ClickHouseSource(t *testing.T) {
	// Seed a memory KlineStore with a deterministic upward ramp and wire it
	// into the server as if it were a ClickHouse instance.
	srv := newBacktestServer(t)
	store := marketstore.NewMemoryStore()
	ctx := context.Background()
	klines := make([]contracts.Kline, 0, 50)
	baseTS := int64(1_700_000_000_000)
	for i := 0; i < 50; i++ {
		px := 100 + float64(i)
		klines = append(klines, contracts.Kline{
			Venue:     contracts.VenueBinance,
			Symbol:    "BTCUSDT",
			Interval:  "1m",
			OpenTime:  baseTS + int64(i)*60_000,
			CloseTime: baseTS + int64(i)*60_000 + 59_999,
			Open:      px, High: px, Low: px, Close: px, Volume: 1,
			Closed: true,
		})
	}
	if err := store.Upsert(ctx, klines); err != nil {
		t.Fatalf("seed: %v", err)
	}
	srv.klines = store

	body := `{
		"strategy_type": "momentum",
		"strategy_params": {"symbol":"BTCUSDT","window_size":5,"breakout_threshold":0.0001,"order_qty":0.1,"cooldown_ms":0},
		"dataset": {
			"source": "clickhouse",
			"symbol": "BTCUSDT",
			"venue": "binance",
			"interval": "1m",
			"num_events": 100,
			"start_ts_ms": 1700000000000,
			"end_ts_ms":   1700003600000
		},
		"start_equity": 10000
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backtests", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.HandleCreateBacktest(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rec BacktestRecord
	_ = json.Unmarshal(rr.Body.Bytes(), &rec)
	if rec.Status != BacktestStatusDone {
		t.Fatalf("status=%s err=%s", rec.Status, rec.Error)
	}
	if rec.Result == nil || rec.Result.Events != 50 {
		t.Fatalf("events=%d want 50 (want all stored klines)", rec.Result.Events)
	}
}

func TestHandleCreateBacktest_ClickHouseSourceNotConfigured(t *testing.T) {
	srv := newBacktestServer(t) // klines == nil
	body := `{
		"strategy_type": "momentum",
		"strategy_params": {"symbol":"BTCUSDT","window_size":5,"breakout_threshold":0.001,"order_qty":0.1,"cooldown_ms":0},
		"dataset": {"source":"clickhouse","symbol":"BTCUSDT","venue":"binance","interval":"1m","num_events":10,"start_ts_ms":1,"end_ts_ms":100},
		"start_equity": 1000
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backtests", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.HandleCreateBacktest(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rec BacktestRecord
	_ = json.Unmarshal(rr.Body.Bytes(), &rec)
	if rec.Status != BacktestStatusFailed {
		t.Fatalf("status=%s want failed", rec.Status)
	}
	if !strings.Contains(rec.Error, "not configured") {
		t.Fatalf("error=%q", rec.Error)
	}
}

// End-to-end round-trip: POST, parse id, GET by id, confirm payload shape.
func TestHandleBacktest_PostThenGetRoundTrip(t *testing.T) {
	srv := newBacktestServer(t)

	body, err := json.Marshal(BacktestRequest{
		StrategyType: "momentum",
		StrategyParams: json.RawMessage(`{
			"symbol":"BTCUSDT","window_size":10,"breakout_threshold":0.0005,"order_qty":0.1,"cooldown_ms":0
		}`),
		Dataset: BacktestDatasetSpec{
			Source:          "synthetic",
			Symbol:          "BTCUSDT",
			NumEvents:       100,
			Seed:            42,
			StartPrice:      50000,
			VolatilityBps:   20,
			TrendBpsPerStep: 5,
		},
		StartEquity: 10000,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/backtests", bytes.NewReader(body)).WithContext(context.Background())
	postRR := httptest.NewRecorder()
	srv.HandleCreateBacktest(postRR, postReq)
	if postRR.Code != http.StatusCreated {
		t.Fatalf("post status=%d body=%s", postRR.Code, postRR.Body.String())
	}
	var created BacktestRecord
	if err := json.Unmarshal(postRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode post: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/backtests/"+created.ID, nil)
	getRR := httptest.NewRecorder()
	srv.HandleGetBacktest(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var fetched BacktestRecord
	if err := json.Unmarshal(getRR.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("fetched id=%s want %s", fetched.ID, created.ID)
	}
	if fetched.Status != BacktestStatusDone {
		t.Fatalf("status=%s want done", fetched.Status)
	}
	if fetched.Result == nil || len(fetched.Result.Equity) == 0 {
		t.Fatal("result/equity missing")
	}
}
