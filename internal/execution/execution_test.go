package execution

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"quant-system/internal/adapter"
	"quant-system/internal/risk"
	"quant-system/internal/strategy"
)

type fakeGateway struct {
	placeCalls  int
	cancelCalls int
	queryCalls  int
	placeErrs   []error
	cancelErrs  []error
	queryErrs   []error
	queryStatus adapter.VenueOrderStatus
}

type noQueryGateway struct{}

func (g *noQueryGateway) PlaceOrder(_ context.Context, req adapter.VenueOrderRequest) (adapter.VenueOrderAck, error) {
	return adapter.VenueOrderAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  "venue-order-no-query",
		Status:        "ack",
	}, nil
}

func (g *noQueryGateway) CancelOrder(_ context.Context, req adapter.VenueCancelRequest) (adapter.VenueCancelAck, error) {
	return adapter.VenueCancelAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  req.VenueOrderID,
		Status:        "canceled",
	}, nil
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
		VenueOrderID:  "venue-order-1",
		Status:        "ack",
	}, nil
}

func (g *fakeGateway) CancelOrder(_ context.Context, req adapter.VenueCancelRequest) (adapter.VenueCancelAck, error) {
	g.cancelCalls++
	if len(g.cancelErrs) > 0 {
		err := g.cancelErrs[0]
		g.cancelErrs = g.cancelErrs[1:]
		return adapter.VenueCancelAck{}, err
	}
	return adapter.VenueCancelAck{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  req.VenueOrderID,
		Status:        "canceled",
	}, nil
}

func (g *fakeGateway) QueryOrder(_ context.Context, req adapter.VenueOrderQueryRequest) (adapter.VenueOrderStatus, error) {
	g.queryCalls++
	if len(g.queryErrs) > 0 {
		err := g.queryErrs[0]
		g.queryErrs = g.queryErrs[1:]
		return adapter.VenueOrderStatus{}, err
	}
	if g.queryStatus != (adapter.VenueOrderStatus{}) {
		return g.queryStatus, nil
	}
	return adapter.VenueOrderStatus{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  req.VenueOrderID,
		Symbol:        req.Symbol,
		Status:        "live",
		FilledQty:     0,
		AvgPrice:      0,
	}, nil
}

func TestSubmitRejectDecision(t *testing.T) {
	gw := &fakeGateway{}
	exec, err := NewInMemoryExecutor(gw)
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}

	_, err = exec.Submit(context.Background(), risk.RiskDecision{
		Decision: risk.DecisionReject,
		Intent: strategy.OrderIntent{
			IntentID: "intent-1",
			Symbol:   "BTC-USDT",
			Side:     "buy",
			Price:    62000,
			Quantity: 0.1,
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrRejectedDecision) {
		t.Fatalf("expected ErrRejectedDecision, got %v", err)
	}
	if gw.placeCalls != 0 {
		t.Fatalf("expected no place calls, got %d", gw.placeCalls)
	}
}

func TestSubmitIdempotent(t *testing.T) {
	gw := &fakeGateway{}
	exec, err := NewInMemoryExecutor(gw)
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}

	decision := risk.RiskDecision{
		Decision: risk.DecisionAllow,
		Intent: strategy.OrderIntent{
			IntentID: "intent-idempotent",
			Symbol:   "BTC-USDT",
			Side:     "buy",
			Price:    62000,
			Quantity: 0.1,
		},
	}
	first, err := exec.Submit(context.Background(), decision)
	if err != nil {
		t.Fatalf("Submit() first error = %v", err)
	}
	second, err := exec.Submit(context.Background(), decision)
	if err != nil {
		t.Fatalf("Submit() second error = %v", err)
	}

	if gw.placeCalls != 1 {
		t.Fatalf("expected 1 place call, got %d", gw.placeCalls)
	}
	if first.ClientOrderID != second.ClientOrderID {
		t.Fatalf("expected same client order id, got %s vs %s", first.ClientOrderID, second.ClientOrderID)
	}
	if !second.IdempotentHit {
		t.Fatalf("expected idempotent hit on second submit")
	}
}

func TestCancel(t *testing.T) {
	gw := &fakeGateway{}
	exec, err := NewInMemoryExecutor(gw)
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}

	result, err := exec.Cancel(context.Background(), CancelIntent{
		ClientOrderID: "cid-1",
		VenueOrderID:  "vo-1",
		Symbol:        "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if result.Status != "canceled" {
		t.Fatalf("unexpected cancel status: %s", result.Status)
	}
	if gw.cancelCalls != 1 {
		t.Fatalf("expected 1 cancel call, got %d", gw.cancelCalls)
	}
}

func TestSubmitRetryableGatewayErrorRetriesAndSucceeds(t *testing.T) {
	gw := &fakeGateway{
		placeErrs: []error{
			fmt.Errorf("%w: temporary", adapter.ErrGatewayRetryable),
		},
	}
	exec, err := NewInMemoryExecutor(gw, ExecutorConfig{
		GatewayMaxRetries:  2,
		GatewayBackoffBase: time.Millisecond,
		GatewayBackoffMax:  2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	exec.sleepWithCtx = func(context.Context, time.Duration) error { return nil }

	result, err := exec.Submit(context.Background(), risk.RiskDecision{
		Decision: risk.DecisionAllow,
		Intent: strategy.OrderIntent{
			IntentID: "intent-retry-success",
			Symbol:   "BTC-USDT",
			Side:     "buy",
			Price:    62000,
			Quantity: 0.1,
		},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if result.VenueOrderID == "" {
		t.Fatalf("expected venue order id, got %+v", result)
	}
	if gw.placeCalls != 2 {
		t.Fatalf("expected 2 place calls, got %d", gw.placeCalls)
	}
}

func TestSubmitNonRetryableGatewayErrorNoRetry(t *testing.T) {
	gw := &fakeGateway{
		placeErrs: []error{
			fmt.Errorf("%w: invalid request", adapter.ErrGatewayNonRetryable),
		},
	}
	exec, err := NewInMemoryExecutor(gw, ExecutorConfig{
		GatewayMaxRetries:  3,
		GatewayBackoffBase: time.Millisecond,
		GatewayBackoffMax:  2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	exec.sleepWithCtx = func(context.Context, time.Duration) error { return nil }

	_, err = exec.Submit(context.Background(), risk.RiskDecision{
		Decision: risk.DecisionAllow,
		Intent: strategy.OrderIntent{
			IntentID: "intent-non-retryable",
			Symbol:   "BTC-USDT",
			Side:     "buy",
			Price:    62000,
			Quantity: 0.1,
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrGatewayNonRetryableFailure) {
		t.Fatalf("expected ErrGatewayNonRetryableFailure, got %v", err)
	}
	if gw.placeCalls != 1 {
		t.Fatalf("expected 1 place call, got %d", gw.placeCalls)
	}
}

func TestSubmitRetryableGatewayErrorExhausted(t *testing.T) {
	gw := &fakeGateway{
		placeErrs: []error{
			fmt.Errorf("%w: timeout", adapter.ErrGatewayRetryable),
			fmt.Errorf("%w: timeout", adapter.ErrGatewayRetryable),
			fmt.Errorf("%w: timeout", adapter.ErrGatewayRetryable),
		},
	}
	exec, err := NewInMemoryExecutor(gw, ExecutorConfig{
		GatewayMaxRetries:  1,
		GatewayBackoffBase: time.Millisecond,
		GatewayBackoffMax:  2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	exec.sleepWithCtx = func(context.Context, time.Duration) error { return nil }

	_, err = exec.Submit(context.Background(), risk.RiskDecision{
		Decision: risk.DecisionAllow,
		Intent: strategy.OrderIntent{
			IntentID: "intent-retry-exhausted",
			Symbol:   "BTC-USDT",
			Side:     "buy",
			Price:    62000,
			Quantity: 0.1,
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrGatewayRetryableFailure) {
		t.Fatalf("expected ErrGatewayRetryableFailure, got %v", err)
	}
	if !errors.Is(err, ErrGatewayRetryExhausted) {
		t.Fatalf("expected ErrGatewayRetryExhausted, got %v", err)
	}
	if gw.placeCalls != 2 {
		t.Fatalf("expected 2 place calls (1 retry), got %d", gw.placeCalls)
	}
}

func TestCancelRetryableGatewayErrorRetries(t *testing.T) {
	gw := &fakeGateway{
		cancelErrs: []error{
			fmt.Errorf("%w: transient", adapter.ErrGatewayRetryable),
		},
	}
	exec, err := NewInMemoryExecutor(gw, ExecutorConfig{
		GatewayMaxRetries:  2,
		GatewayBackoffBase: time.Millisecond,
		GatewayBackoffMax:  2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	exec.sleepWithCtx = func(context.Context, time.Duration) error { return nil }

	result, err := exec.Cancel(context.Background(), CancelIntent{
		ClientOrderID: "cid-cancel",
		Symbol:        "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if result.Status != "canceled" {
		t.Fatalf("unexpected cancel result: %+v", result)
	}
	if gw.cancelCalls != 2 {
		t.Fatalf("expected 2 cancel calls, got %d", gw.cancelCalls)
	}
}

func TestReconcileSuccess(t *testing.T) {
	gw := &fakeGateway{
		queryStatus: adapter.VenueOrderStatus{
			ClientOrderID: "cid-r1",
			VenueOrderID:  "vo-r1",
			Symbol:        "BTC-USDT",
			Status:        "partially_filled",
			FilledQty:     0.2,
			AvgPrice:      62000,
		},
	}
	exec, err := NewInMemoryExecutor(gw, ExecutorConfig{
		GatewayMaxRetries:  1,
		GatewayBackoffBase: time.Millisecond,
		GatewayBackoffMax:  2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	exec.sleepWithCtx = func(context.Context, time.Duration) error { return nil }

	result, err := exec.Reconcile(context.Background(), ReconcileIntent{
		ClientOrderID: "cid-r1",
		Symbol:        "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.Status != "partially_filled" || result.FilledQty != 0.2 || result.AvgPrice != 62000 {
		t.Fatalf("unexpected reconcile result: %+v", result)
	}
	if gw.queryCalls != 1 {
		t.Fatalf("expected 1 query call, got %d", gw.queryCalls)
	}
}

func TestReconcileRetryableThenSuccess(t *testing.T) {
	gw := &fakeGateway{
		queryErrs: []error{
			fmt.Errorf("%w: timeout", adapter.ErrGatewayRetryable),
		},
		queryStatus: adapter.VenueOrderStatus{
			ClientOrderID: "cid-r2",
			VenueOrderID:  "vo-r2",
			Symbol:        "BTC-USDT",
			Status:        "live",
		},
	}
	exec, err := NewInMemoryExecutor(gw, ExecutorConfig{
		GatewayMaxRetries:  2,
		GatewayBackoffBase: time.Millisecond,
		GatewayBackoffMax:  2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	exec.sleepWithCtx = func(context.Context, time.Duration) error { return nil }

	_, err = exec.Reconcile(context.Background(), ReconcileIntent{
		ClientOrderID: "cid-r2",
		Symbol:        "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if gw.queryCalls != 2 {
		t.Fatalf("expected 2 query calls, got %d", gw.queryCalls)
	}
}

func TestReconcileInvalidRequest(t *testing.T) {
	gw := &fakeGateway{}
	exec, err := NewInMemoryExecutor(gw)
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}

	_, err = exec.Reconcile(context.Background(), ReconcileIntent{
		ClientOrderID: "",
		VenueOrderID:  "",
		Symbol:        "",
	})
	if !errors.Is(err, ErrInvalidReconcileRequest) {
		t.Fatalf("expected ErrInvalidReconcileRequest, got %v", err)
	}
}

func TestReconcileUnsupportedGateway(t *testing.T) {
	gw := &noQueryGateway{}
	exec, err := NewInMemoryExecutor(gw)
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}

	_, err = exec.Reconcile(context.Background(), ReconcileIntent{
		ClientOrderID: "cid-r3",
		Symbol:        "BTC-USDT",
	})
	if !errors.Is(err, ErrOrderQueryUnsupported) {
		t.Fatalf("expected ErrOrderQueryUnsupported, got %v", err)
	}
}

func TestReconcileUsesQueryRetryBudgetOverride(t *testing.T) {
	gw := &fakeGateway{
		queryErrs: []error{
			fmt.Errorf("%w: timeout", adapter.ErrGatewayRetryable),
		},
		queryStatus: adapter.VenueOrderStatus{
			ClientOrderID: "cid-r4",
			VenueOrderID:  "vo-r4",
			Symbol:        "BTC-USDT",
			Status:        "live",
		},
	}
	exec, err := NewInMemoryExecutor(gw, ExecutorConfig{
		GatewayMaxRetries:      0,
		GatewayQueryMaxRetries: 1,
		GatewayBackoffBase:     time.Millisecond,
		GatewayBackoffMax:      2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	exec.sleepWithCtx = func(context.Context, time.Duration) error { return nil }

	_, err = exec.Reconcile(context.Background(), ReconcileIntent{
		ClientOrderID: "cid-r4",
		Symbol:        "BTC-USDT",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if gw.queryCalls != 2 {
		t.Fatalf("expected 2 query calls with query retry override, got %d", gw.queryCalls)
	}
}
