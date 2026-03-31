package position

import (
	"context"
	"errors"
	"testing"
)

func TestApplyFillBuyThenSell(t *testing.T) {
	ledger := NewInMemoryLedger()

	_, err := ledger.ApplyFill(context.Background(), TradeFillEvent{
		TradeID:   "t-1",
		AccountID: "acc-1",
		Symbol:    "BTC-USDT",
		Side:      "buy",
		FillQty:   2,
		FillPrice: 100,
		Fee:       2,
	})
	if err != nil {
		t.Fatalf("buy fill error = %v", err)
	}

	snapshot, err := ledger.ApplyFill(context.Background(), TradeFillEvent{
		TradeID:   "t-2",
		AccountID: "acc-1",
		Symbol:    "BTC-USDT",
		Side:      "sell",
		FillQty:   1,
		FillPrice: 120,
		Fee:       1,
	})
	if err != nil {
		t.Fatalf("sell fill error = %v", err)
	}

	if snapshot.Quantity != 1 {
		t.Fatalf("expected quantity 1, got %f", snapshot.Quantity)
	}
	if snapshot.RealizedPnL <= 0 {
		t.Fatalf("expected positive realized pnl, got %f", snapshot.RealizedPnL)
	}
}

func TestApplyFillIdempotent(t *testing.T) {
	ledger := NewInMemoryLedger()
	fill := TradeFillEvent{
		TradeID:   "t-idem",
		AccountID: "acc-1",
		Symbol:    "BTC-USDT",
		Side:      "buy",
		FillQty:   1,
		FillPrice: 100,
	}

	first, err := ledger.ApplyFill(context.Background(), fill)
	if err != nil {
		t.Fatalf("first apply error = %v", err)
	}
	second, err := ledger.ApplyFill(context.Background(), fill)
	if err != nil {
		t.Fatalf("second apply error = %v", err)
	}

	if first.Quantity != second.Quantity || first.AvgCost != second.AvgCost {
		t.Fatalf("expected idempotent snapshot, first=%+v second=%+v", first, second)
	}
}

func TestApplyFillRejectSellExceedsPosition(t *testing.T) {
	ledger := NewInMemoryLedger()
	_, err := ledger.ApplyFill(context.Background(), TradeFillEvent{
		TradeID:   "t-oversell",
		AccountID: "acc-1",
		Symbol:    "BTC-USDT",
		Side:      "sell",
		FillQty:   1,
		FillPrice: 100,
	})
	if err == nil {
		t.Fatal("expected insufficient position error")
	}
	if !errors.Is(err, ErrInsufficientPosition) {
		t.Fatalf("expected ErrInsufficientPosition, got %v", err)
	}
}
