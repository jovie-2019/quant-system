package risk

import (
	"context"
	"testing"

	"quant-system/internal/strategy"
)

func TestEvaluateAllow(t *testing.T) {
	engine := NewInMemoryEngine(Config{
		MaxOrderQty:    2,
		MaxOrderAmount: 200000,
		AllowedSymbols: map[string]struct{}{
			"BTC-USDT": {},
		},
	})

	intent := strategy.OrderIntent{
		IntentID: "intent-allow-1",
		Symbol:   "BTC-USDT",
		Price:    62000,
		Quantity: 1,
	}
	decision := engine.Evaluate(context.Background(), intent)
	if decision.Decision != DecisionAllow {
		t.Fatalf("expected allow, got %s", decision.Decision)
	}
}

func TestEvaluateRejectForQtyLimit(t *testing.T) {
	engine := NewInMemoryEngine(Config{
		MaxOrderQty: 1,
	})

	intent := strategy.OrderIntent{
		IntentID: "intent-reject-qty",
		Symbol:   "BTC-USDT",
		Price:    62000,
		Quantity: 1.5,
	}
	decision := engine.Evaluate(context.Background(), intent)
	if decision.Decision != DecisionReject {
		t.Fatalf("expected reject, got %s", decision.Decision)
	}
	if decision.RuleID != "risk.qty.max" {
		t.Fatalf("expected rule risk.qty.max, got %s", decision.RuleID)
	}
}

func TestEvaluateIdempotentByIntentID(t *testing.T) {
	engine := NewInMemoryEngine(Config{
		MaxOrderQty: 1,
	})

	intent := strategy.OrderIntent{
		IntentID: "intent-idempotent-1",
		Symbol:   "BTC-USDT",
		Price:    62000,
		Quantity: 1.5,
	}
	first := engine.Evaluate(context.Background(), intent)

	engine.SetConfig(Config{
		MaxOrderQty: 10,
	})
	second := engine.Evaluate(context.Background(), intent)

	if first.Decision != second.Decision || first.RuleID != second.RuleID {
		t.Fatalf("expected idempotent decision, first=%+v second=%+v", first, second)
	}
}

func TestEvaluateFailClosedOnInvalidInput(t *testing.T) {
	engine := NewInMemoryEngine(Config{})

	intent := strategy.OrderIntent{
		IntentID: "intent-invalid",
		Symbol:   "BTC-USDT",
		Price:    0,
		Quantity: 1,
	}
	decision := engine.Evaluate(context.Background(), intent)
	if decision.Decision != DecisionReject {
		t.Fatalf("expected reject, got %s", decision.Decision)
	}
	if decision.RuleID != "risk.price.invalid" {
		t.Fatalf("expected rule risk.price.invalid, got %s", decision.RuleID)
	}
}
