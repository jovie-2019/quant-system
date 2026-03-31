package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSymbols(t *testing.T) {
	got := parseSymbols(" btc-usdt ,ETH-USDT,,  sol-usdt ")
	want := []string{"BTC-USDT", "ETH-USDT", "SOL-USDT"}
	if len(got) != len(want) {
		t.Fatalf("unexpected symbols len: got=%d want=%d values=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected symbol at %d: got=%s want=%s", i, got[i], want[i])
		}
	}
}

func TestCreateMarketStreamUnsupportedVenue(t *testing.T) {
	_, err := createMarketStream("unknown-venue")
	if err == nil {
		t.Fatal("expected unsupported venue error")
	}
}

func TestCreateMarketStreamSupportedVenue(t *testing.T) {
	if _, err := createMarketStream("binance"); err != nil {
		t.Fatalf("expected binance market stream, got error: %v", err)
	}
	if _, err := createMarketStream("okx"); err != nil {
		t.Fatalf("expected okx market stream, got error: %v", err)
	}
}

func TestHTTPHandlerHealthAndMetrics(t *testing.T) {
	h := newHTTPHandler()

	healthReq := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	healthResp := httptest.NewRecorder()
	h.ServeHTTP(healthResp, healthReq)
	if healthResp.Code != http.StatusOK {
		t.Fatalf("health status mismatch: got=%d want=%d", healthResp.Code, http.StatusOK)
	}
	if !strings.Contains(healthResp.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected health body: %s", healthResp.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsResp := httptest.NewRecorder()
	h.ServeHTTP(metricsResp, metricsReq)
	if metricsResp.Code != http.StatusOK {
		t.Fatalf("metrics status mismatch: got=%d want=%d", metricsResp.Code, http.StatusOK)
	}
	if !strings.Contains(metricsResp.Body.String(), "engine_controlapi_http_requests_total") {
		t.Fatalf("expected prometheus metrics body, got: %s", metricsResp.Body.String())
	}
}
