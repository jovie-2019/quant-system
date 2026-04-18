package core

import (
	"context"
	"testing"

	"quant-system/pkg/contracts"
)

func TestMemorySink_RecordsAndResets(t *testing.T) {
	sink := NewMemorySink()
	ctx := context.Background()

	_ = sink.PublishRiskDecision(ctx, "acct", contracts.RiskDecision{RuleID: "r1"})
	_ = sink.PublishOrderLifecycle(ctx, "acct", contracts.OrderLifecycleEvent{ClientOrderID: "c1", State: "ack"})
	_ = sink.PublishTradeFill(ctx, "acct", contracts.TradeFillEvent{TradeID: "t1", FillQty: 1, FillPrice: 100})

	if got := len(sink.Decisions()); got != 1 {
		t.Fatalf("decisions=%d want=1", got)
	}
	if got := len(sink.Lifecycle()); got != 1 {
		t.Fatalf("lifecycle=%d want=1", got)
	}
	if got := len(sink.Fills()); got != 1 {
		t.Fatalf("fills=%d want=1", got)
	}

	// Snapshot returns copies, mutating caller's slice must not affect internal state.
	fills := sink.Fills()
	fills[0].FillPrice = -1
	if sink.Fills()[0].FillPrice != 100 {
		t.Fatal("MemorySink returned internal slice, expected copy")
	}

	sink.Reset()
	if len(sink.Decisions()) != 0 || len(sink.Lifecycle()) != 0 || len(sink.Fills()) != 0 {
		t.Fatal("Reset did not clear buffers")
	}
}

func TestNopSink_NeverFails(t *testing.T) {
	var sink EventSink = NopSink{}
	ctx := context.Background()
	if err := sink.PublishRiskDecision(ctx, "a", contracts.RiskDecision{}); err != nil {
		t.Fatal(err)
	}
	if err := sink.PublishOrderLifecycle(ctx, "a", contracts.OrderLifecycleEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := sink.PublishTradeFill(ctx, "a", contracts.TradeFillEvent{}); err != nil {
		t.Fatal(err)
	}
}
