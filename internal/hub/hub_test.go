package hub

import (
	"testing"
	"time"

	"quant-system/internal/adapter"
	"quant-system/internal/book"
	"quant-system/internal/normalizer"
)

func TestPublishSubscribe(t *testing.T) {
	h := NewInMemoryHub()
	ch, unsubscribe := h.Subscribe("s1", []string{"BTC-USDT"}, 8)
	defer unsubscribe()

	event := normalizer.MarketNormalizedEvent{
		Venue:  adapter.VenueBinance,
		Symbol: "BTC-USDT",
		BidPX:  62000.1,
		AskPX:  62000.2,
	}

	h.Publish(event)

	select {
	case got := <-ch:
		if got.Symbol != "BTC-USDT" || got.Venue != adapter.VenueBinance {
			t.Fatalf("unexpected event: %#v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestDropCount(t *testing.T) {
	h := NewInMemoryHub()
	_, unsubscribe := h.Subscribe("s1", []string{"BTC-USDT"}, 1)
	defer unsubscribe()

	event := normalizer.MarketNormalizedEvent{
		Venue:  adapter.VenueBinance,
		Symbol: "BTC-USDT",
	}

	h.Publish(event)
	h.Publish(event)

	if h.DropCount() == 0 {
		t.Fatalf("expected drop count > 0")
	}
}

func TestBookSnapshotAndGapCount(t *testing.T) {
	h := NewInMemoryHub()

	h.Publish(normalizer.MarketNormalizedEvent{
		Venue:    adapter.VenueBinance,
		Symbol:   "BTC-USDT",
		Sequence: 100,
		BidPX:    62000.1,
		BidSZ:    1.2,
		AskPX:    62000.2,
		AskSZ:    1.1,
		LastPX:   62000.15,
	})
	h.Publish(normalizer.MarketNormalizedEvent{
		Venue:    adapter.VenueBinance,
		Symbol:   "BTC-USDT",
		Sequence: 102, // introduce gap
		BidPX:    62001.1,
		BidSZ:    1.0,
		AskPX:    62001.2,
		AskSZ:    0.9,
		LastPX:   62001.15,
	})

	snapshot, ok := h.GetBookSnapshot(book.VenueSymbol{
		Venue:  adapter.VenueBinance,
		Symbol: "BTC-USDT",
	})
	if !ok {
		t.Fatal("expected book snapshot")
	}
	if snapshot.BestBidPX != 62001.1 || snapshot.BestAskPX != 62001.2 {
		t.Fatalf("unexpected book snapshot: %+v", snapshot)
	}
	if !snapshot.Stale {
		t.Fatalf("expected stale snapshot because of seq gap")
	}
	if h.BookSeqGapCount() == 0 {
		t.Fatalf("expected seq gap count > 0")
	}
}
