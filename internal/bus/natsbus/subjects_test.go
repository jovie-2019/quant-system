package natsbus

import "testing"

func TestSubjectBuilders(t *testing.T) {
	if got := SubjectMarketNormalizedSpot("binance", "BTC-USDT"); got != "market.normalized.spot.binance.BTC-USDT" {
		t.Fatalf("unexpected market subject: %s", got)
	}
	if got := SubjectStrategyIntent("s1"); got != "strategy.intent.s1" {
		t.Fatalf("unexpected strategy subject: %s", got)
	}
	if got := SubjectRiskDecision("acc1"); got != "risk.decision.acc1" {
		t.Fatalf("unexpected risk subject: %s", got)
	}
	if got := SubjectOrderLifecycle("acc1", "BTC-USDT"); got != "order.lifecycle.acc1.BTC-USDT" {
		t.Fatalf("unexpected order subject: %s", got)
	}
	if got := SubjectTradeFill("acc1", "BTC-USDT"); got != "trade.fill.acc1.BTC-USDT" {
		t.Fatalf("unexpected fill subject: %s", got)
	}
}
