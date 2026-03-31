package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"quant-system/internal/adapter"
	"quant-system/internal/execution"
	"quant-system/internal/hub"
	"quant-system/internal/normalizer"
	"quant-system/internal/orderfsm"
	"quant-system/internal/position"
	"quant-system/internal/risk"
	"quant-system/internal/strategy"
	"quant-system/pkg/contracts"
)

type fixedStrategy struct {
	id       string
	intentID string
	qty      float64
}

func (s fixedStrategy) ID() string { return s.id }

func (s fixedStrategy) OnMarket(evt contracts.MarketNormalizedEvent) []strategy.OrderIntent {
	return []strategy.OrderIntent{
		{
			IntentID:    s.intentID,
			Symbol:      evt.Symbol,
			Side:        "buy",
			Price:       evt.AskPX,
			Quantity:    s.qty,
			TimeInForce: "IOC",
		},
	}
}

type fakeGateway struct {
	placeCalls int
	placeErrs  []error
}

func (g *fakeGateway) PlaceOrder(_ context.Context, req adapter.VenueOrderRequest) (adapter.VenueOrderAck, error) {
	g.placeCalls++
	if len(g.placeErrs) > 0 {
		err := g.placeErrs[0]
		g.placeErrs = g.placeErrs[1:]
		return adapter.VenueOrderAck{}, err
	}
	return adapter.VenueOrderAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  "vo-" + req.ClientOrderID,
		Status:        "ack",
	}, nil
}

func (g *fakeGateway) CancelOrder(_ context.Context, req adapter.VenueCancelRequest) (adapter.VenueCancelAck, error) {
	return adapter.VenueCancelAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  req.VenueOrderID,
		Status:        "canceled",
	}, nil
}

func TestPipelineHappyPath(t *testing.T) {
	ctx := context.Background()
	allowCount := 0
	rejectCount := 0

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty: 10,
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

	runtime, err := strategy.NewInMemoryRuntime(func(ctx context.Context, intent strategy.OrderIntent) error {
		decision := riskEngine.Evaluate(ctx, intent)
		if decision.Decision == risk.DecisionReject {
			rejectCount++
			return nil
		}
		allowCount++
		submit, err := exec.Submit(ctx, decision)
		if err != nil {
			return err
		}

		_, err = fsm.Apply(orderfsm.Event{
			ClientOrderID: submit.ClientOrderID,
			VenueOrderID:  submit.VenueOrderID,
			Symbol:        intent.Symbol,
			State:         orderfsm.StateAck,
			FilledQty:     0,
			AvgPrice:      0,
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
	if err := runtime.Register(fixedStrategy{id: "s1", intentID: "intent-happy", qty: 0.1}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	marketHub := hub.NewInMemoryHub()
	subCh, unsubscribe := marketHub.Subscribe("s1", []string{"BTC-USDT"}, 8)
	defer unsubscribe()

	n := normalizer.NewJSONNormalizer()
	evt, err := n.NormalizeMarket(adapter.RawMarketEvent{
		Venue:  adapter.VenueBinance,
		Symbol: "BTC-USDT",
		Payload: []byte(`{
			"bid_px":"62000.1",
			"bid_sz":"1.2",
			"ask_px":"62000.2",
			"ask_sz":"0.8",
			"last_px":"62000.15",
			"seq":"1",
			"ts":"1700000000000"
		}`),
	})
	if err != nil {
		t.Fatalf("NormalizeMarket() error = %v", err)
	}
	marketHub.Publish(evt)

	select {
	case incoming := <-subCh:
		if err := runtime.HandleMarket(ctx, incoming); err != nil {
			t.Fatalf("HandleMarket() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting market event")
	}

	if allowCount != 1 || rejectCount != 0 {
		t.Fatalf("unexpected decisions allow=%d reject=%d", allowCount, rejectCount)
	}
	if gateway.placeCalls != 1 {
		t.Fatalf("expected 1 place order call, got %d", gateway.placeCalls)
	}

	order, ok := fsm.Get("cid-intent-happy")
	if !ok || order.State != orderfsm.StateFilled {
		t.Fatalf("expected filled order, got %+v found=%v", order, ok)
	}
	pos, ok := ledger.Get("acc-1", "BTC-USDT")
	if !ok || pos.Quantity <= 0 {
		t.Fatalf("expected positive position, got %+v found=%v", pos, ok)
	}
}

func TestPipelineRejectPath(t *testing.T) {
	ctx := context.Background()
	allowCount := 0
	rejectCount := 0

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty: 0.05,
		AllowedSymbols: map[string]struct{}{
			"BTC-USDT": {},
		},
	})
	gateway := &fakeGateway{}
	exec, err := execution.NewInMemoryExecutor(gateway)
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}

	runtime, err := strategy.NewInMemoryRuntime(func(ctx context.Context, intent strategy.OrderIntent) error {
		decision := riskEngine.Evaluate(ctx, intent)
		if decision.Decision == risk.DecisionReject {
			rejectCount++
			return nil
		}
		allowCount++
		_, err := exec.Submit(ctx, decision)
		return err
	})
	if err != nil {
		t.Fatalf("NewInMemoryRuntime() error = %v", err)
	}
	if err := runtime.Register(fixedStrategy{id: "s1", intentID: "intent-reject", qty: 0.1}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	marketHub := hub.NewInMemoryHub()
	subCh, unsubscribe := marketHub.Subscribe("s1", []string{"BTC-USDT"}, 8)
	defer unsubscribe()

	n := normalizer.NewJSONNormalizer()
	evt, err := n.NormalizeMarket(adapter.RawMarketEvent{
		Venue:  adapter.VenueBinance,
		Symbol: "BTC-USDT",
		Payload: []byte(`{
			"bid_px":"62000.1",
			"bid_sz":"1.2",
			"ask_px":"62000.2",
			"ask_sz":"0.8",
			"last_px":"62000.15"
		}`),
	})
	if err != nil {
		t.Fatalf("NormalizeMarket() error = %v", err)
	}
	marketHub.Publish(evt)

	select {
	case incoming := <-subCh:
		if err := runtime.HandleMarket(ctx, incoming); err != nil {
			t.Fatalf("HandleMarket() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting market event")
	}

	if allowCount != 0 || rejectCount != 1 {
		t.Fatalf("unexpected decisions allow=%d reject=%d", allowCount, rejectCount)
	}
	if gateway.placeCalls != 0 {
		t.Fatalf("expected 0 place order calls, got %d", gateway.placeCalls)
	}
}

func TestPipelineRetryableGatewayExhausted(t *testing.T) {
	ctx := context.Background()

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty: 10,
		AllowedSymbols: map[string]struct{}{
			"BTC-USDT": {},
		},
	})
	gateway := &fakeGateway{
		placeErrs: []error{
			fmt.Errorf("%w: timeout", adapter.ErrGatewayRetryable),
			fmt.Errorf("%w: timeout", adapter.ErrGatewayRetryable),
		},
	}
	exec, err := execution.NewInMemoryExecutor(gateway, execution.ExecutorConfig{
		GatewayPlaceMaxRetries: 1,
		GatewayBackoffBase:     time.Millisecond,
		GatewayBackoffMax:      time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
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
		t.Fatalf("NewInMemoryRuntime() error = %v", err)
	}
	if err := runtime.Register(fixedStrategy{id: "s1", intentID: "intent-retry-exhausted", qty: 0.1}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	marketHub := hub.NewInMemoryHub()
	subCh, unsubscribe := marketHub.Subscribe("s1", []string{"BTC-USDT"}, 8)
	defer unsubscribe()

	n := normalizer.NewJSONNormalizer()
	evt, err := n.NormalizeMarket(adapter.RawMarketEvent{
		Venue:  adapter.VenueBinance,
		Symbol: "BTC-USDT",
		Payload: []byte(`{
			"bid_px":"62000.1",
			"bid_sz":"1.2",
			"ask_px":"62000.2",
			"ask_sz":"0.8",
			"last_px":"62000.15"
		}`),
	})
	if err != nil {
		t.Fatalf("NormalizeMarket() error = %v", err)
	}
	marketHub.Publish(evt)

	select {
	case incoming := <-subCh:
		err := runtime.HandleMarket(ctx, incoming)
		if err == nil {
			t.Fatal("expected retryable failure error")
		}
		if !errors.Is(err, execution.ErrGatewayRetryableFailure) {
			t.Fatalf("expected ErrGatewayRetryableFailure, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting market event")
	}

	if gateway.placeCalls != 2 {
		t.Fatalf("expected 2 place order calls (1 retry), got %d", gateway.placeCalls)
	}
}

func TestPipelineNonRetryableGatewayError(t *testing.T) {
	ctx := context.Background()

	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty: 10,
		AllowedSymbols: map[string]struct{}{
			"BTC-USDT": {},
		},
	})
	gateway := &fakeGateway{
		placeErrs: []error{
			fmt.Errorf("%w: invalid order request", adapter.ErrGatewayNonRetryable),
		},
	}
	exec, err := execution.NewInMemoryExecutor(gateway, execution.ExecutorConfig{
		GatewayPlaceMaxRetries: 3,
		GatewayBackoffBase:     time.Millisecond,
		GatewayBackoffMax:      time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
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
		t.Fatalf("NewInMemoryRuntime() error = %v", err)
	}
	if err := runtime.Register(fixedStrategy{id: "s1", intentID: "intent-non-retryable", qty: 0.1}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	marketHub := hub.NewInMemoryHub()
	subCh, unsubscribe := marketHub.Subscribe("s1", []string{"BTC-USDT"}, 8)
	defer unsubscribe()

	n := normalizer.NewJSONNormalizer()
	evt, err := n.NormalizeMarket(adapter.RawMarketEvent{
		Venue:  adapter.VenueBinance,
		Symbol: "BTC-USDT",
		Payload: []byte(`{
			"bid_px":"62000.1",
			"bid_sz":"1.2",
			"ask_px":"62000.2",
			"ask_sz":"0.8",
			"last_px":"62000.15"
		}`),
	})
	if err != nil {
		t.Fatalf("NormalizeMarket() error = %v", err)
	}
	marketHub.Publish(evt)

	select {
	case incoming := <-subCh:
		err := runtime.HandleMarket(ctx, incoming)
		if err == nil {
			t.Fatal("expected non-retryable failure error")
		}
		if !errors.Is(err, execution.ErrGatewayNonRetryableFailure) {
			t.Fatalf("expected ErrGatewayNonRetryableFailure, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting market event")
	}

	if gateway.placeCalls != 1 {
		t.Fatalf("expected 1 place order call (no retry), got %d", gateway.placeCalls)
	}
}
