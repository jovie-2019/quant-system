package natsbus

import (
	"context"
	"testing"

	"quant-system/pkg/contracts"
)

func TestPublishHelpersRejectMissingAccount(t *testing.T) {
	ctx := context.Background()
	client := &Client{}

	if err := PublishStrategyIntent(ctx, client, contracts.OrderIntent{}, nil); err == nil {
		t.Fatal("expected error for empty strategy id in strategy intent publish")
	}
	if err := PublishRiskDecision(ctx, client, "", contracts.RiskDecision{}, nil); err == nil {
		t.Fatal("expected error for empty account in risk decision publish")
	}
	if err := PublishOrderLifecycle(ctx, client, "", contracts.OrderLifecycleEvent{}, nil); err == nil {
		t.Fatal("expected error for empty account in order lifecycle publish")
	}
	if err := PublishTradeFill(ctx, client, "", contracts.TradeFillEvent{}, nil); err == nil {
		t.Fatal("expected error for empty account in trade fill publish")
	}
}
