package book

import (
	"strings"
	"sync"
	"time"

	"quant-system/pkg/contracts"
)

type VenueSymbol struct {
	Venue  contracts.Venue
	Symbol string
}

type Level struct {
	Price float64
	Size  float64
}

type BookSnapshot struct {
	Venue       contracts.Venue
	Symbol      string
	Bids        []Level
	Asks        []Level
	Sequence    int64
	Stale       bool
	StaleSince  int64
	UpdatedMS   int64
	BestBidPX   float64
	BestBidSZ   float64
	BestAskPX   float64
	BestAskSZ   float64
	LastTradePX float64
}

type ApplyResult struct {
	Snapshot   BookSnapshot
	SeqGap     bool
	Duplicate  bool
	OutOfOrder bool
}

type Engine interface {
	Apply(evt contracts.MarketNormalizedEvent) (ApplyResult, error)
	Snapshot(key VenueSymbol) (BookSnapshot, bool)
	MarkResynced(key VenueSymbol)
	SeqGapCount() uint64
}

type InMemoryEngine struct {
	mu          sync.RWMutex
	snapshots   map[VenueSymbol]BookSnapshot
	seqGapCount uint64
}

func NewInMemoryEngine() *InMemoryEngine {
	return &InMemoryEngine{
		snapshots: make(map[VenueSymbol]BookSnapshot),
	}
}

func (e *InMemoryEngine) Apply(evt contracts.MarketNormalizedEvent) (ApplyResult, error) {
	key := VenueSymbol{
		Venue:  evt.Venue,
		Symbol: normalizeSymbol(evt.Symbol),
	}

	nowMS := time.Now().UnixMilli()

	e.mu.Lock()
	defer e.mu.Unlock()

	current, exists := e.snapshots[key]
	if !exists {
		snapshot := newSnapshotFromEvent(evt, nowMS)
		e.snapshots[key] = snapshot
		return ApplyResult{Snapshot: snapshot}, nil
	}

	result := ApplyResult{}

	if evt.Sequence > 0 && current.Sequence > 0 {
		switch {
		case evt.Sequence == current.Sequence:
			result.Duplicate = true
			result.Snapshot = current
			return result, nil
		case evt.Sequence < current.Sequence:
			result.OutOfOrder = true
			result.Snapshot = current
			return result, nil
		case evt.Sequence > current.Sequence+1:
			result.SeqGap = true
			current.Stale = true
			if current.StaleSince == 0 {
				current.StaleSince = nowMS
			}
			e.seqGapCount++
		}
	}

	current.Bids = []Level{{Price: evt.BidPX, Size: evt.BidSZ}}
	current.Asks = []Level{{Price: evt.AskPX, Size: evt.AskSZ}}
	current.BestBidPX = evt.BidPX
	current.BestBidSZ = evt.BidSZ
	current.BestAskPX = evt.AskPX
	current.BestAskSZ = evt.AskSZ
	current.LastTradePX = evt.LastPX
	if evt.Sequence > 0 {
		current.Sequence = evt.Sequence
	}
	current.UpdatedMS = nowMS
	current.Venue = evt.Venue
	current.Symbol = key.Symbol

	e.snapshots[key] = current
	result.Snapshot = current
	return result, nil
}

func (e *InMemoryEngine) Snapshot(key VenueSymbol) (BookSnapshot, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	key.Symbol = normalizeSymbol(key.Symbol)
	snapshot, ok := e.snapshots[key]
	return snapshot, ok
}

func (e *InMemoryEngine) MarkResynced(key VenueSymbol) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key.Symbol = normalizeSymbol(key.Symbol)
	snapshot, ok := e.snapshots[key]
	if !ok {
		return
	}
	snapshot.Stale = false
	snapshot.StaleSince = 0
	e.snapshots[key] = snapshot
}

func (e *InMemoryEngine) SeqGapCount() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.seqGapCount
}

func newSnapshotFromEvent(evt contracts.MarketNormalizedEvent, nowMS int64) BookSnapshot {
	return BookSnapshot{
		Venue:       evt.Venue,
		Symbol:      normalizeSymbol(evt.Symbol),
		Bids:        []Level{{Price: evt.BidPX, Size: evt.BidSZ}},
		Asks:        []Level{{Price: evt.AskPX, Size: evt.AskSZ}},
		Sequence:    evt.Sequence,
		Stale:       false,
		StaleSince:  0,
		UpdatedMS:   nowMS,
		BestBidPX:   evt.BidPX,
		BestBidSZ:   evt.BidSZ,
		BestAskPX:   evt.AskPX,
		BestAskSZ:   evt.AskSZ,
		LastTradePX: evt.LastPX,
	}
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
