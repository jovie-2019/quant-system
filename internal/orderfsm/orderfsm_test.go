package orderfsm

import (
	"errors"
	"testing"
)

func TestApplyLegalTransition(t *testing.T) {
	fsm := NewInMemoryStateMachine()

	_, err := fsm.Apply(Event{
		ClientOrderID: "cid-1",
		Symbol:        "BTC-USDT",
		State:         StateNew,
	})
	if err != nil {
		t.Fatalf("apply new error = %v", err)
	}

	order, err := fsm.Apply(Event{
		ClientOrderID: "cid-1",
		Symbol:        "BTC-USDT",
		State:         StateAck,
	})
	if err != nil {
		t.Fatalf("apply ack error = %v", err)
	}
	if order.State != StateAck {
		t.Fatalf("expected state ack, got %s", order.State)
	}
	if order.StateVersion != 2 {
		t.Fatalf("expected version 2, got %d", order.StateVersion)
	}
}

func TestApplyIllegalTransition(t *testing.T) {
	fsm := NewInMemoryStateMachine()
	_, err := fsm.Apply(Event{
		ClientOrderID: "cid-2",
		Symbol:        "BTC-USDT",
		State:         StateCanceled,
	})
	if err != nil {
		t.Fatalf("apply canceled error = %v", err)
	}

	_, err = fsm.Apply(Event{
		ClientOrderID: "cid-2",
		Symbol:        "BTC-USDT",
		State:         StateAck,
	})
	if err == nil {
		t.Fatal("expected illegal transition error")
	}
	if !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("expected ErrIllegalTransition, got %v", err)
	}
}

func TestApplyIdempotentEvent(t *testing.T) {
	fsm := NewInMemoryStateMachine()
	first, err := fsm.Apply(Event{
		ClientOrderID: "cid-3",
		Symbol:        "BTC-USDT",
		State:         StateAck,
		FilledQty:     0,
		AvgPrice:      0,
	})
	if err != nil {
		t.Fatalf("first apply error = %v", err)
	}

	second, err := fsm.Apply(Event{
		ClientOrderID: "cid-3",
		Symbol:        "BTC-USDT",
		State:         StateAck,
		FilledQty:     0,
		AvgPrice:      0,
	})
	if err != nil {
		t.Fatalf("second apply error = %v", err)
	}
	if second.StateVersion != first.StateVersion {
		t.Fatalf("expected idempotent version, first=%d second=%d", first.StateVersion, second.StateVersion)
	}
}
