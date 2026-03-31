package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestExposePrometheusIncludesRiskAndExecutionMetrics(t *testing.T) {
	ResetForTest()

	ObserveRiskEvaluation("allow", 800*time.Microsecond)
	ObserveRiskEvaluation("reject", 1500*time.Microsecond)
	ObserveExecutionSubmit("success", 2*time.Millisecond)
	ObserveExecutionSubmit("error", 3*time.Millisecond)
	ObserveExecutionGateway("place_order", "retry")
	ObserveExecutionGateway("place_order", "success")

	out := ExposePrometheus()
	if !strings.Contains(out, "engine_risk_decision_total") {
		t.Fatalf("expected risk decision metric, got: %s", out)
	}
	if !strings.Contains(out, `decision="reject"`) {
		t.Fatalf("expected reject decision label, got: %s", out)
	}
	if !strings.Contains(out, "engine_execution_submit_total") {
		t.Fatalf("expected execution submit metric, got: %s", out)
	}
	if !strings.Contains(out, `outcome="error"`) {
		t.Fatalf("expected execution error outcome label, got: %s", out)
	}
	if !strings.Contains(out, "engine_execution_gateway_events_total") {
		t.Fatalf("expected execution gateway metric, got: %s", out)
	}
	if !strings.Contains(out, `engine_execution_gateway_events_total{operation="place_order",result="retry"} 1`) {
		t.Fatalf("expected execution gateway retry series, got: %s", out)
	}
}

func TestExposePrometheusIncludesTTLCacheAndMomentumMetrics(t *testing.T) {
	ResetForTest()

	ObserveTTLCacheGet("risk_decision", true)
	ObserveTTLCacheGet("risk_decision", false)
	ObserveTTLCacheEviction("risk_decision", "capacity")
	ObserveTTLCachePurge("risk_decision", 2)
	ObserveTTLCacheSize("risk_decision", 3)

	ObserveMomentumEvaluation("btc-usdt", "buy_signal", 2*time.Millisecond)
	ObserveMomentumSignal("btc-usdt", "buy")
	ObserveMarketIngest("binance", "published")
	ObserveMarketIngest("okx", "normalize_error")

	out := ExposePrometheus()
	if !strings.Contains(out, "engine_ttlcache_get_total") {
		t.Fatalf("expected ttlcache get metric, got: %s", out)
	}
	if !strings.Contains(out, `engine_ttlcache_get_total{cache="risk_decision",result="hit"} 1`) {
		t.Fatalf("expected ttlcache hit series, got: %s", out)
	}
	if !strings.Contains(out, `engine_ttlcache_eviction_total{cache="risk_decision",reason="capacity"} 1`) {
		t.Fatalf("expected ttlcache eviction series, got: %s", out)
	}
	if !strings.Contains(out, `engine_ttlcache_purge_total{cache="risk_decision"} 2`) {
		t.Fatalf("expected ttlcache purge series, got: %s", out)
	}
	if !strings.Contains(out, `engine_ttlcache_size{cache="risk_decision"} 3`) {
		t.Fatalf("expected ttlcache size gauge, got: %s", out)
	}

	if !strings.Contains(out, "engine_strategy_momentum_eval_total") {
		t.Fatalf("expected momentum eval metric, got: %s", out)
	}
	if !strings.Contains(out, `engine_strategy_momentum_eval_total{symbol="BTC-USDT",outcome="buy_signal"} 1`) {
		t.Fatalf("expected momentum eval series, got: %s", out)
	}
	if !strings.Contains(out, `engine_strategy_momentum_eval_duration_ms_count{symbol="BTC-USDT"} 1`) {
		t.Fatalf("expected momentum eval duration histogram count, got: %s", out)
	}
	if !strings.Contains(out, `engine_strategy_momentum_signal_total{symbol="BTC-USDT",side="buy"} 1`) {
		t.Fatalf("expected momentum signal series, got: %s", out)
	}

	if !strings.Contains(out, "engine_market_ingest_events_total") {
		t.Fatalf("expected market ingest metric, got: %s", out)
	}
	if !strings.Contains(out, `engine_market_ingest_events_total{venue="binance",result="published"} 1`) {
		t.Fatalf("expected market ingest published series, got: %s", out)
	}
	if !strings.Contains(out, `engine_market_ingest_events_total{venue="okx",result="normalize_error"} 1`) {
		t.Fatalf("expected market ingest normalize_error series, got: %s", out)
	}
}
