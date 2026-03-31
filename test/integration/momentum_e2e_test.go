package integration

import (
	"context"
	"testing"

	"quant-system/internal/execution"
	"quant-system/internal/orderfsm"
	"quant-system/internal/position"
	"quant-system/internal/risk"
	"quant-system/internal/strategy"
	momentum "quant-system/internal/strategy/momentum"
	"quant-system/pkg/contracts"
)

func TestMomentumE2E_FullPipeline(t *testing.T) {
	ctx := context.Background()

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty:    100,
		MaxOrderAmount: 100_000_000,
		AllowedSymbols: map[string]struct{}{
			"BTC-USDT": {},
		},
	})
	gateway := &fakeGateway{}
	exec, err := execution.NewInMemoryExecutor(gateway)
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()

	var lastSubmit execution.SubmitResult

	runtime, err := strategy.NewInMemoryRuntime(func(ctx context.Context, intent strategy.OrderIntent) error {
		decision := riskEngine.Evaluate(ctx, intent)
		if decision.Decision == risk.DecisionReject {
			return nil
		}
		submit, err := exec.Submit(ctx, decision)
		if err != nil {
			return err
		}
		lastSubmit = submit

		_, err = fsm.Apply(orderfsm.Event{
			ClientOrderID: submit.ClientOrderID,
			VenueOrderID:  submit.VenueOrderID,
			Symbol:        intent.Symbol,
			State:         orderfsm.StateAck,
		})
		if err != nil {
			return err
		}
		_, err = fsm.Apply(orderfsm.Event{
			ClientOrderID: submit.ClientOrderID,
			VenueOrderID:  submit.VenueOrderID,
			Symbol:        intent.Symbol,
			State:         orderfsm.StateFilled,
			FilledQty:     intent.Quantity,
			AvgPrice:      intent.Price,
		})
		if err != nil {
			return err
		}
		_, err = ledger.ApplyFill(ctx, position.TradeFillEvent{
			TradeID:   "trade-" + intent.IntentID,
			AccountID: "acc-1",
			Symbol:    intent.Symbol,
			Side:      intent.Side,
			FillQty:   intent.Quantity,
			FillPrice: intent.Price,
		})
		return err
	})
	if err != nil {
		t.Fatalf("NewInMemoryRuntime() error = %v", err)
	}

	strat := momentum.New(momentum.Config{
		Symbol:            "BTC-USDT",
		WindowSize:        5,
		BreakoutThreshold: 0.001,
		OrderQty:          0.5,
		Cooldown:          0,
	})
	if err := runtime.Register(strat); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Step 1: Feed 5 stable prices to fill window — no orders expected.
	for i := 0; i < 5; i++ {
		evt := contracts.MarketNormalizedEvent{
			Symbol: "BTC-USDT",
			LastPX: 60000,
			AskPX:  60001,
			BidPX:  59999,
		}
		if err := runtime.HandleMarket(ctx, evt); err != nil {
			t.Fatalf("HandleMarket stable[%d] error = %v", i, err)
		}
	}
	if gateway.placeCalls != 0 {
		t.Fatalf("expected 0 orders during window fill, got %d", gateway.placeCalls)
	}

	// Step 2: Feed breakout price — expect BUY.
	breakoutEvt := contracts.MarketNormalizedEvent{
		Symbol: "BTC-USDT",
		LastPX: 60100,
		AskPX:  60101,
		BidPX:  60099,
	}
	if err := runtime.HandleMarket(ctx, breakoutEvt); err != nil {
		t.Fatalf("HandleMarket breakout error = %v", err)
	}
	if gateway.placeCalls != 1 {
		t.Fatalf("expected 1 order after breakout, got %d", gateway.placeCalls)
	}

	// Verify FSM state.
	order, ok := fsm.Get(lastSubmit.ClientOrderID)
	if !ok {
		t.Fatal("expected order in FSM")
	}
	if order.State != orderfsm.StateFilled {
		t.Fatalf("expected filled state, got %s", order.State)
	}

	// Verify position.
	pos, ok := ledger.Get("acc-1", "BTC-USDT")
	if !ok || pos.Quantity != 0.5 {
		t.Fatalf("expected position qty=0.5, got %+v found=%v", pos, ok)
	}

	// Step 3: Fill window at higher level, then feed breakdown — expect SELL.
	for i := 0; i < 5; i++ {
		evt := contracts.MarketNormalizedEvent{
			Symbol: "BTC-USDT",
			LastPX: 60100,
			AskPX:  60101,
			BidPX:  60099,
		}
		if err := runtime.HandleMarket(ctx, evt); err != nil {
			t.Fatalf("HandleMarket high[%d] error = %v", i, err)
		}
	}

	breakdownEvt := contracts.MarketNormalizedEvent{
		Symbol: "BTC-USDT",
		LastPX: 59900,
		AskPX:  59901,
		BidPX:  59899,
	}
	if err := runtime.HandleMarket(ctx, breakdownEvt); err != nil {
		t.Fatalf("HandleMarket breakdown error = %v", err)
	}
	if gateway.placeCalls != 2 {
		t.Fatalf("expected 2 orders (buy + sell), got %d", gateway.placeCalls)
	}

	// Verify position after sell: should be 0.
	pos, ok = ledger.Get("acc-1", "BTC-USDT")
	if !ok {
		t.Fatal("expected position to exist")
	}
	if pos.Quantity != 0 {
		t.Fatalf("expected position qty=0 after sell, got %f", pos.Quantity)
	}
}

func TestMomentumE2E_StableNoSignal(t *testing.T) {
	ctx := context.Background()

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty:    100,
		MaxOrderAmount: 100_000_000,
		AllowedSymbols: map[string]struct{}{"BTC-USDT": {}},
	})
	gateway := &fakeGateway{}
	exec, err := execution.NewInMemoryExecutor(gateway)
	if err != nil {
		t.Fatal(err)
	}

	runtime, err := strategy.NewInMemoryRuntime(func(ctx context.Context, intent strategy.OrderIntent) error {
		decision := riskEngine.Evaluate(ctx, intent)
		if decision.Decision == risk.DecisionReject {
			return nil
		}
		_, err := exec.Submit(ctx, decision)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	strat := momentum.New(momentum.Config{
		Symbol:            "BTC-USDT",
		WindowSize:        5,
		BreakoutThreshold: 0.001,
		OrderQty:          0.5,
		Cooldown:          0,
	})
	if err := runtime.Register(strat); err != nil {
		t.Fatal(err)
	}

	// Feed 20 stable prices — no orders expected.
	for i := 0; i < 20; i++ {
		evt := contracts.MarketNormalizedEvent{
			Symbol: "BTC-USDT",
			LastPX: 60000,
			AskPX:  60001,
			BidPX:  59999,
		}
		if err := runtime.HandleMarket(ctx, evt); err != nil {
			t.Fatalf("HandleMarket stable[%d] error = %v", i, err)
		}
	}
	if gateway.placeCalls != 0 {
		t.Fatalf("expected 0 orders for stable prices, got %d", gateway.placeCalls)
	}
}
