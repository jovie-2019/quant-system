package controlapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quant-system/internal/obs/metrics"
	"quant-system/internal/risk"
)

type fakeRiskSetter struct {
	last risk.Config
}

func (f *fakeRiskSetter) SetConfig(config risk.Config) {
	f.last = config
}

func TestHealthEndpoint(t *testing.T) {
	metrics.ResetForTest()
	srv := NewServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected response: %s", rr.Body.String())
	}
}

func TestMetricsEndpoint(t *testing.T) {
	metrics.ResetForTest()
	srv := NewServer(nil)

	healthReq := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	healthResp := httptest.NewRecorder()
	srv.ServeHTTP(healthResp, healthReq)
	if healthResp.Code != http.StatusOK {
		t.Fatalf("health expected 200, got %d", healthResp.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsResp := httptest.NewRecorder()
	srv.ServeHTTP(metricsResp, metricsReq)
	if metricsResp.Code != http.StatusOK {
		t.Fatalf("metrics expected 200, got %d", metricsResp.Code)
	}

	body := metricsResp.Body.String()
	if !strings.Contains(body, "engine_controlapi_http_requests_total") {
		t.Fatalf("expected request counter metric, body=%s", body)
	}
	if !strings.Contains(body, `path="/api/v1/health"`) {
		t.Fatalf("expected health path metric, body=%s", body)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/strategies/s1/start", nil)
	startResp := httptest.NewRecorder()
	srv.ServeHTTP(startResp, startReq)
	if startResp.Code != http.StatusOK {
		t.Fatalf("strategy start expected 200, got %d", startResp.Code)
	}

	metricsResp2 := httptest.NewRecorder()
	srv.ServeHTTP(metricsResp2, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body2 := metricsResp2.Body.String()
	if !strings.Contains(body2, `path="/api/v1/strategies/{id}/start"`) {
		t.Fatalf("expected normalized strategy path metric, body=%s", body2)
	}
}

func TestStrategyStartStopAndConfig(t *testing.T) {
	srv := NewServer(nil)

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/strategies/s1/start", nil)
	startResp := httptest.NewRecorder()
	srv.ServeHTTP(startResp, startReq)
	if startResp.Code != http.StatusOK {
		t.Fatalf("start expected 200, got %d", startResp.Code)
	}

	state, ok := srv.GetStrategyState("s1")
	if !ok || !state.Running {
		t.Fatalf("expected strategy running state, got %+v", state)
	}

	cfgReq := httptest.NewRequest(http.MethodPut, "/api/v1/strategies/s1/config", strings.NewReader(`{"config":{"threshold":1.5}}`))
	cfgResp := httptest.NewRecorder()
	srv.ServeHTTP(cfgResp, cfgReq)
	if cfgResp.Code != http.StatusOK {
		t.Fatalf("config expected 200, got %d", cfgResp.Code)
	}

	state, ok = srv.GetStrategyState("s1")
	if !ok {
		t.Fatalf("expected strategy state")
	}
	if state.Config["threshold"] != 1.5 {
		t.Fatalf("expected threshold 1.5, got %+v", state.Config["threshold"])
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/api/v1/strategies/s1/stop", nil)
	stopResp := httptest.NewRecorder()
	srv.ServeHTTP(stopResp, stopReq)
	if stopResp.Code != http.StatusOK {
		t.Fatalf("stop expected 200, got %d", stopResp.Code)
	}

	state, ok = srv.GetStrategyState("s1")
	if !ok || state.Running {
		t.Fatalf("expected strategy stopped state, got %+v", state)
	}
}

func TestRiskConfigEndpoint(t *testing.T) {
	setter := &fakeRiskSetter{}
	srv := NewServer(setter)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/risk/config", strings.NewReader(`{
		"max_order_qty": 2,
		"max_order_amount": 150000,
		"allowed_symbols": ["btc-usdt", "eth-usdt"]
	}`))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	got := srv.GetRiskConfig()
	if got.MaxOrderQty != 2 || got.MaxOrderAmount != 150000 {
		t.Fatalf("unexpected risk config: %+v", got)
	}

	if _, ok := setter.last.AllowedSymbols["BTC-USDT"]; !ok {
		t.Fatalf("expected normalized symbol BTC-USDT")
	}
}
