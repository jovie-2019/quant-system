package marketdata

import (
	"log/slog"
	"sync"

	"quant-system/pkg/contracts"
)

// DepthManager is a thread-safe in-memory store for order book depth snapshots.
type DepthManager struct {
	mu        sync.RWMutex
	snapshots map[string]contracts.DepthSnapshot // key: "venue:symbol"
	logger    *slog.Logger
}

// NewDepthManager creates a new DepthManager.
func NewDepthManager(logger *slog.Logger) *DepthManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &DepthManager{
		snapshots: make(map[string]contracts.DepthSnapshot),
		logger:    logger,
	}
}

func depthKey(venue, symbol string) string {
	return venue + ":" + symbol
}

// Update replaces the current depth snapshot for a symbol.
func (m *DepthManager) Update(snapshot contracts.DepthSnapshot) {
	key := depthKey(string(snapshot.Venue), snapshot.Symbol)

	m.mu.Lock()
	m.snapshots[key] = snapshot
	m.mu.Unlock()
}

// Get returns the current depth snapshot for a venue+symbol.
func (m *DepthManager) Get(venue, symbol string) (contracts.DepthSnapshot, bool) {
	key := depthKey(venue, symbol)

	m.mu.RLock()
	defer m.mu.RUnlock()

	snap, ok := m.snapshots[key]
	return snap, ok
}

// Spread returns the bid-ask spread for a venue+symbol (ask[0].price - bid[0].price).
func (m *DepthManager) Spread(venue, symbol string) (float64, bool) {
	snap, ok := m.Get(venue, symbol)
	if !ok || len(snap.Bids) == 0 || len(snap.Asks) == 0 {
		return 0, false
	}
	return snap.Asks[0].Price - snap.Bids[0].Price, true
}

// MidPrice returns (bestBid + bestAsk) / 2 for a venue+symbol.
func (m *DepthManager) MidPrice(venue, symbol string) (float64, bool) {
	snap, ok := m.Get(venue, symbol)
	if !ok || len(snap.Bids) == 0 || len(snap.Asks) == 0 {
		return 0, false
	}
	return (snap.Bids[0].Price + snap.Asks[0].Price) / 2, true
}
