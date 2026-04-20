package adminapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quant-system/internal/optimizer"
	_ "quant-system/internal/strategy/momentum"
)

func newOptimizationServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		logger:        slog.Default(),
		backtests:     NewBacktestStore(20),
		optimizations: NewOptimizationStore(20),
	}
}

func TestHandleCreateOptimization_Grid(t *testing.T) {
	srv := newOptimizationServer(t)

	body := `{
		"strategy_type": "momentum",
		"base_params": {"symbol":"BTCUSDT","order_qty":0.1,"time_in_force":"IOC","cooldown_ms":0},
		"params": [
			{"name": "window_size",        "type": "int",   "min": 5,      "max": 25,    "step": 10},
			{"name": "breakout_threshold", "type": "float", "min": 0.0002, "max": 0.002, "step": 0.0009}
		],
		"dataset": {
			"source": "synthetic",
			"symbol": "BTCUSDT",
			"num_events": 500,
			"seed": 42,
			"start_price": 20000,
			"volatility_bps": 20,
			"trend_bps_per_step": 10
		},
		"start_equity": 10000,
		"algorithm":    "grid",
		"max_trials":   30
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/optimizations", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.HandleCreateOptimization(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rec OptimizationRecord
	if err := json.Unmarshal(rr.Body.Bytes(), &rec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec.Status != OptimizationStatusDone {
		t.Fatalf("status=%s err=%s", rec.Status, rec.Error)
	}
	if rec.Result == nil {
		t.Fatal("result nil")
	}
	if len(rec.Result.Trials) == 0 {
		t.Fatal("no trials executed")
	}
	if rec.Result.Algorithm != optimizer.AlgorithmGrid {
		t.Fatalf("algo=%s want grid", rec.Result.Algorithm)
	}
	if _, ok := rec.Result.Importance["window_size"]; !ok {
		t.Fatalf("importance missing window_size: %+v", rec.Result.Importance)
	}

	// Subsequent GET should return the same record.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/optimizations/"+rec.ID, nil)
	getRR := httptest.NewRecorder()
	srv.HandleGetOptimization(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRR.Code, getRR.Body.String())
	}
}

func TestHandleCreateOptimization_InvalidBody(t *testing.T) {
	srv := newOptimizationServer(t)
	cases := []struct {
		name string
		body string
	}{
		{"missing strategy_type",
			`{"base_params":{},"params":[{"name":"x","type":"int","min":0,"max":1,"step":1}],"dataset":{"symbol":"X","num_events":10}}`},
		{"no params",
			`{"strategy_type":"momentum","base_params":{},"params":[],"dataset":{"symbol":"X","num_events":10}}`},
		{"zero events",
			`{"strategy_type":"momentum","base_params":{},"params":[{"name":"a","type":"int","min":1,"max":2,"step":1}],"dataset":{"symbol":"X","num_events":0}}`},
		{"max trials too high",
			`{"strategy_type":"momentum","base_params":{},"params":[{"name":"a","type":"int","min":1,"max":2,"step":1}],"dataset":{"symbol":"X","num_events":10},"max_trials":10000}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/optimizations", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()
			srv.HandleCreateOptimization(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleCreateOptimization_UnknownStrategyCapturedInRecord(t *testing.T) {
	srv := newOptimizationServer(t)
	body := `{
		"strategy_type": "no-such-strategy",
		"base_params": {},
		"params": [{"name":"x","type":"int","min":1,"max":2,"step":1}],
		"dataset": {"source":"synthetic","symbol":"X","num_events":50}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/optimizations", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.HandleCreateOptimization(rr, req)
	// Request validates OK; optimiser runs trials, but each trial records
	// a BuildError for the unknown strategy type — record status stays done.
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rec OptimizationRecord
	_ = json.Unmarshal(rr.Body.Bytes(), &rec)
	if rec.Status != OptimizationStatusDone {
		t.Fatalf("status=%s want done", rec.Status)
	}
	if rec.Result == nil || len(rec.Result.Trials) == 0 {
		t.Fatal("expected trials (with BuildError) to be recorded")
	}
	if rec.Result.Trials[0].BuildError == "" {
		t.Fatalf("expected BuildError set, got %+v", rec.Result.Trials[0])
	}
}

func TestHandleGetOptimization_NotFound(t *testing.T) {
	srv := newOptimizationServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/optimizations/opt_missing", nil)
	rr := httptest.NewRecorder()
	srv.HandleGetOptimization(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleListOptimizations(t *testing.T) {
	srv := newOptimizationServer(t)

	body := `{
		"strategy_type": "momentum",
		"base_params": {"symbol":"BTCUSDT","order_qty":0.1,"time_in_force":"IOC","cooldown_ms":0},
		"params": [{"name":"window_size","type":"int","min":5,"max":15,"step":5}],
		"dataset": {"source":"synthetic","symbol":"BTCUSDT","num_events":120,"seed":1,"start_price":100,"volatility_bps":10},
		"start_equity": 1000
	}`
	run := func() string {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/optimizations", strings.NewReader(body))
		rr := httptest.NewRecorder()
		srv.HandleCreateOptimization(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create status=%d", rr.Code)
		}
		var rec OptimizationRecord
		_ = json.Unmarshal(rr.Body.Bytes(), &rec)
		return rec.ID
	}
	firstID := run()
	secondID := run()

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/optimizations?limit=10", nil)
	listRR := httptest.NewRecorder()
	srv.HandleListOptimizations(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var resp struct {
		Items []OptimizationRecord `json:"items"`
		Count int                  `json:"count"`
	}
	_ = json.Unmarshal(listRR.Body.Bytes(), &resp)
	if resp.Count < 2 {
		t.Fatalf("count=%d want >=2", resp.Count)
	}
	// Newest first.
	if resp.Items[0].ID != secondID || resp.Items[1].ID != firstID {
		t.Fatalf("order wrong: %s, %s", resp.Items[0].ID, resp.Items[1].ID)
	}
}
