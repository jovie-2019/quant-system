package replay

import (
	"context"
	"testing"

	"quant-system/internal/orderfsm"
	"quant-system/internal/position"
)

func TestOrderAndPositionReplayDeterministic(t *testing.T) {
	events := []orderfsm.Event{
		{ClientOrderID: "cid-replay-1", Symbol: "BTC-USDT", State: orderfsm.StateNew},
		{ClientOrderID: "cid-replay-1", Symbol: "BTC-USDT", State: orderfsm.StateAck},
		{ClientOrderID: "cid-replay-1", Symbol: "BTC-USDT", State: orderfsm.StateFilled, FilledQty: 1, AvgPrice: 100},
	}
	fills := []position.TradeFillEvent{
		{TradeID: "trade-r-1", AccountID: "acc-r", Symbol: "BTC-USDT", Side: "buy", FillQty: 1, FillPrice: 100},
	}

	firstOrder, firstPos := replayRun(t, events, fills)
	secondOrder, secondPos := replayRun(t, events, fills)

	if firstOrder.State != secondOrder.State || firstOrder.FilledQty != secondOrder.FilledQty {
		t.Fatalf("order replay mismatch first=%+v second=%+v", firstOrder, secondOrder)
	}
	if firstPos.Quantity != secondPos.Quantity || firstPos.AvgCost != secondPos.AvgCost || firstPos.RealizedPnL != secondPos.RealizedPnL {
		t.Fatalf("position replay mismatch first=%+v second=%+v", firstPos, secondPos)
	}
}

func replayRun(t *testing.T, events []orderfsm.Event, fills []position.TradeFillEvent) (orderfsm.Order, position.PositionSnapshot) {
	t.Helper()

	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()

	var latestOrder orderfsm.Order
	for _, evt := range events {
		order, err := fsm.Apply(evt)
		if err != nil {
			t.Fatalf("fsm apply error: %v", err)
		}
		latestOrder = order
	}
	for _, fill := range fills {
		_, err := ledger.ApplyFill(context.Background(), fill)
		if err != nil {
			t.Fatalf("ledger apply fill error: %v", err)
		}
	}

	pos, ok := ledger.Get("acc-r", "BTC-USDT")
	if !ok {
		t.Fatalf("expected position snapshot")
	}
	return latestOrder, pos
}
