package replay

import (
	"testing"

	"quant-system/internal/book"
	"quant-system/pkg/contracts"
)

func TestBookReplaySequenceBehavior(t *testing.T) {
	engine := book.NewInMemoryEngine()
	events := []contracts.MarketNormalizedEvent{
		{
			Venue:    contracts.VenueBinance,
			Symbol:   "BTC-USDT",
			Sequence: 100,
			BidPX:    62000.1,
			BidSZ:    1.1,
			AskPX:    62000.2,
			AskSZ:    1.0,
			LastPX:   62000.15,
		},
		{
			Venue:    contracts.VenueBinance,
			Symbol:   "BTC-USDT",
			Sequence: 101,
			BidPX:    62001.1,
			BidSZ:    1.2,
			AskPX:    62001.2,
			AskSZ:    1.1,
			LastPX:   62001.15,
		},
		{
			Venue:    contracts.VenueBinance,
			Symbol:   "BTC-USDT",
			Sequence: 101, // duplicate
			BidPX:    63000.1,
			BidSZ:    9.9,
			AskPX:    63000.2,
			AskSZ:    9.9,
			LastPX:   63000.15,
		},
		{
			Venue:    contracts.VenueBinance,
			Symbol:   "BTC-USDT",
			Sequence: 99, // out-of-order
			BidPX:    61000.1,
			BidSZ:    0.9,
			AskPX:    61000.2,
			AskSZ:    0.8,
			LastPX:   61000.15,
		},
		{
			Venue:    contracts.VenueBinance,
			Symbol:   "BTC-USDT",
			Sequence: 104, // gap
			BidPX:    62004.1,
			BidSZ:    1.3,
			AskPX:    62004.2,
			AskSZ:    1.2,
			LastPX:   62004.15,
		},
	}

	for _, evt := range events {
		if _, err := engine.Apply(evt); err != nil {
			t.Fatalf("book apply error: %v", err)
		}
	}

	snap, ok := engine.Snapshot(book.VenueSymbol{
		Venue:  contracts.VenueBinance,
		Symbol: "BTC-USDT",
	})
	if !ok {
		t.Fatal("expected final snapshot")
	}
	if snap.Sequence != 104 {
		t.Fatalf("expected sequence 104, got %d", snap.Sequence)
	}
	if !snap.Stale {
		t.Fatalf("expected stale due to sequence gap")
	}
	if engine.SeqGapCount() != 1 {
		t.Fatalf("expected one seq gap, got %d", engine.SeqGapCount())
	}
}
