package book

import (
	"testing"

	"quant-system/pkg/contracts"
)

func TestApplyContiguousSequence(t *testing.T) {
	engine := NewInMemoryEngine()

	first, err := engine.Apply(contracts.MarketNormalizedEvent{
		Venue:    contracts.VenueBinance,
		Symbol:   "BTC-USDT",
		Sequence: 10,
		BidPX:    62000.1,
		BidSZ:    1.2,
		AskPX:    62000.2,
		AskSZ:    0.9,
		LastPX:   62000.15,
	})
	if err != nil {
		t.Fatalf("apply first error: %v", err)
	}
	if first.SeqGap || first.Duplicate || first.OutOfOrder {
		t.Fatalf("unexpected flags in first apply: %+v", first)
	}

	second, err := engine.Apply(contracts.MarketNormalizedEvent{
		Venue:    contracts.VenueBinance,
		Symbol:   "BTC-USDT",
		Sequence: 11,
		BidPX:    62001.1,
		BidSZ:    1.0,
		AskPX:    62001.2,
		AskSZ:    0.8,
		LastPX:   62001.15,
	})
	if err != nil {
		t.Fatalf("apply second error: %v", err)
	}
	if second.SeqGap || second.Duplicate || second.OutOfOrder {
		t.Fatalf("unexpected flags in second apply: %+v", second)
	}

	snap, ok := engine.Snapshot(VenueSymbol{Venue: contracts.VenueBinance, Symbol: "BTC-USDT"})
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.Sequence != 11 || snap.BestBidPX != 62001.1 || snap.BestAskPX != 62001.2 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}

func TestApplySequenceGapAndResync(t *testing.T) {
	engine := NewInMemoryEngine()

	_, _ = engine.Apply(contracts.MarketNormalizedEvent{
		Venue:    contracts.VenueBinance,
		Symbol:   "BTC-USDT",
		Sequence: 20,
		BidPX:    62000,
		BidSZ:    1,
		AskPX:    62001,
		AskSZ:    1,
		LastPX:   62000.5,
	})

	result, err := engine.Apply(contracts.MarketNormalizedEvent{
		Venue:    contracts.VenueBinance,
		Symbol:   "BTC-USDT",
		Sequence: 22,
		BidPX:    62002,
		BidSZ:    1,
		AskPX:    62003,
		AskSZ:    1,
		LastPX:   62002.5,
	})
	if err != nil {
		t.Fatalf("apply gap error: %v", err)
	}
	if !result.SeqGap {
		t.Fatalf("expected seq gap flag")
	}
	if !result.Snapshot.Stale {
		t.Fatalf("expected stale snapshot")
	}
	if engine.SeqGapCount() != 1 {
		t.Fatalf("expected seq gap count=1, got %d", engine.SeqGapCount())
	}

	key := VenueSymbol{Venue: contracts.VenueBinance, Symbol: "BTC-USDT"}
	engine.MarkResynced(key)
	snap, ok := engine.Snapshot(key)
	if !ok {
		t.Fatal("expected snapshot after resync")
	}
	if snap.Stale || snap.StaleSince != 0 {
		t.Fatalf("expected non-stale snapshot after resync: %+v", snap)
	}
}

func TestApplyDuplicateAndOutOfOrder(t *testing.T) {
	engine := NewInMemoryEngine()

	_, _ = engine.Apply(contracts.MarketNormalizedEvent{
		Venue:    contracts.VenueOKX,
		Symbol:   "ETH-USDT",
		Sequence: 100,
		BidPX:    3100,
		BidSZ:    1,
		AskPX:    3101,
		AskSZ:    1,
		LastPX:   3100.5,
	})

	dup, err := engine.Apply(contracts.MarketNormalizedEvent{
		Venue:    contracts.VenueOKX,
		Symbol:   "ETH-USDT",
		Sequence: 100,
		BidPX:    3200,
		BidSZ:    1,
		AskPX:    3201,
		AskSZ:    1,
		LastPX:   3200.5,
	})
	if err != nil {
		t.Fatalf("apply duplicate error: %v", err)
	}
	if !dup.Duplicate || dup.OutOfOrder || dup.SeqGap {
		t.Fatalf("unexpected duplicate flags: %+v", dup)
	}

	outOfOrder, err := engine.Apply(contracts.MarketNormalizedEvent{
		Venue:    contracts.VenueOKX,
		Symbol:   "ETH-USDT",
		Sequence: 99,
		BidPX:    3300,
		BidSZ:    1,
		AskPX:    3301,
		AskSZ:    1,
		LastPX:   3300.5,
	})
	if err != nil {
		t.Fatalf("apply out-of-order error: %v", err)
	}
	if !outOfOrder.OutOfOrder || outOfOrder.Duplicate || outOfOrder.SeqGap {
		t.Fatalf("unexpected out-of-order flags: %+v", outOfOrder)
	}
}
