package perf

import (
	"context"
	"fmt"
	"testing"
	"time"

	"quant-system/internal/orderfsm"
	"quant-system/internal/risk"
	"quant-system/internal/strategy"
)

func TestRiskEvaluateBaseline(t *testing.T) {
	engine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty: 10,
	})
	const n = 20000

	start := time.Now()
	for i := 0; i < n; i++ {
		intent := strategy.OrderIntent{
			IntentID: fmt.Sprintf("intent-perf-%d", i),
			Symbol:   "BTC-USDT",
			Price:    62000,
			Quantity: 0.1,
		}
		decision := engine.Evaluate(context.Background(), intent)
		if decision.Decision != risk.DecisionAllow {
			t.Fatalf("unexpected reject at %d: %+v", i, decision)
		}
	}
	d := time.Since(start)
	avg := d / n
	t.Logf("risk baseline n=%d total=%s avg/op=%s", n, d, avg)

	if d > 10*time.Second {
		t.Fatalf("risk baseline too slow: %s", d)
	}
}

func TestOrderFSMApplyBaseline(t *testing.T) {
	fsm := orderfsm.NewInMemoryStateMachine()
	const n = 20000

	start := time.Now()
	for i := 0; i < n; i++ {
		cid := fmt.Sprintf("cid-perf-%d", i)
		_, err := fsm.Apply(orderfsm.Event{ClientOrderID: cid, Symbol: "BTC-USDT", State: orderfsm.StateNew})
		if err != nil {
			t.Fatalf("apply new error: %v", err)
		}
		_, err = fsm.Apply(orderfsm.Event{ClientOrderID: cid, Symbol: "BTC-USDT", State: orderfsm.StateAck})
		if err != nil {
			t.Fatalf("apply ack error: %v", err)
		}
		_, err = fsm.Apply(orderfsm.Event{ClientOrderID: cid, Symbol: "BTC-USDT", State: orderfsm.StateFilled, FilledQty: 0.1, AvgPrice: 62000})
		if err != nil {
			t.Fatalf("apply filled error: %v", err)
		}
	}
	d := time.Since(start)
	ops := n * 3
	avg := d / time.Duration(ops)
	t.Logf("orderfsm baseline orders=%d ops=%d total=%s avg/op=%s", n, ops, d, avg)

	if d > 10*time.Second {
		t.Fatalf("orderfsm baseline too slow: %s", d)
	}
}
