package hub

import (
	"strings"
	"sync"

	"quant-system/internal/adapter"
	"quant-system/internal/book"
	"quant-system/internal/normalizer"
)

type VenueSymbol struct {
	Venue  adapter.Venue
	Symbol string
}

type MarketHub interface {
	Publish(evt normalizer.MarketNormalizedEvent)
	Subscribe(strategyID string, symbols []string, buffer int) (<-chan normalizer.MarketNormalizedEvent, func())
	GetSnapshot(key VenueSymbol) (normalizer.MarketNormalizedEvent, bool)
	GetBookSnapshot(key book.VenueSymbol) (book.BookSnapshot, bool)
	DropCount() uint64
	BookSeqGapCount() uint64
}

type subscriber struct {
	id      string
	symbols map[string]struct{}
	ch      chan normalizer.MarketNormalizedEvent
}

type InMemoryHub struct {
	mu          sync.RWMutex
	snapshots   map[VenueSymbol]normalizer.MarketNormalizedEvent
	subscribers map[string]subscriber
	bookEngine  book.Engine
	dropCount   uint64
}

func NewInMemoryHub() *InMemoryHub {
	return &InMemoryHub{
		snapshots:   make(map[VenueSymbol]normalizer.MarketNormalizedEvent),
		subscribers: make(map[string]subscriber),
		bookEngine:  book.NewInMemoryEngine(),
	}
}

func (h *InMemoryHub) Publish(evt normalizer.MarketNormalizedEvent) {
	h.mu.Lock()
	key := VenueSymbol{Venue: evt.Venue, Symbol: evt.Symbol}
	h.snapshots[key] = evt
	if h.bookEngine != nil {
		_, _ = h.bookEngine.Apply(evt)
	}

	targets := make([]subscriber, 0, len(h.subscribers))
	for _, sub := range h.subscribers {
		if _, ok := sub.symbols[normalizeSymbol(evt.Symbol)]; ok {
			targets = append(targets, sub)
		}
	}
	h.mu.Unlock()

	for _, sub := range targets {
		select {
		case sub.ch <- evt:
		default:
			h.mu.Lock()
			h.dropCount++
			h.mu.Unlock()
		}
	}
}

func (h *InMemoryHub) Subscribe(strategyID string, symbols []string, buffer int) (<-chan normalizer.MarketNormalizedEvent, func()) {
	if buffer <= 0 {
		buffer = 64
	}

	symbolSet := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		symbolSet[normalizeSymbol(symbol)] = struct{}{}
	}

	ch := make(chan normalizer.MarketNormalizedEvent, buffer)
	sub := subscriber{
		id:      strategyID,
		symbols: symbolSet,
		ch:      ch,
	}

	h.mu.Lock()
	h.subscribers[strategyID] = sub
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		delete(h.subscribers, strategyID)
		h.mu.Unlock()
	}

	return ch, unsubscribe
}

func (h *InMemoryHub) GetSnapshot(key VenueSymbol) (normalizer.MarketNormalizedEvent, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	evt, ok := h.snapshots[key]
	return evt, ok
}

func (h *InMemoryHub) GetBookSnapshot(key book.VenueSymbol) (book.BookSnapshot, bool) {
	h.mu.RLock()
	engine := h.bookEngine
	h.mu.RUnlock()
	if engine == nil {
		return book.BookSnapshot{}, false
	}
	return engine.Snapshot(key)
}

func (h *InMemoryHub) DropCount() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.dropCount
}

func (h *InMemoryHub) BookSeqGapCount() uint64 {
	h.mu.RLock()
	engine := h.bookEngine
	h.mu.RUnlock()
	if engine == nil {
		return 0
	}
	return engine.SeqGapCount()
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
