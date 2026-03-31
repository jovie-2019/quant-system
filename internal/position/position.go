package position

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"quant-system/internal/obs/ttlcache"
	"quant-system/pkg/contracts"
)

var (
	ErrInvalidFill          = errors.New("position: invalid fill event")
	ErrInsufficientPosition = errors.New("position: insufficient position")
)

type TradeFillEvent = contracts.TradeFillEvent
type PositionSnapshot = contracts.PositionSnapshot

type LedgerConfig struct {
	CacheTTL     time.Duration
	CacheMaxSize int
	Logger       *slog.Logger
}

type PositionLedger interface {
	ApplyFill(ctx context.Context, fill TradeFillEvent) (PositionSnapshot, error)
	Get(accountID, symbol string) (PositionSnapshot, bool)
}

type InMemoryLedger struct {
	mu          sync.RWMutex
	snapshots   map[string]PositionSnapshot
	appliedFill *ttlcache.Cache[struct{}]
	logger      *slog.Logger
}

func NewInMemoryLedger(cfgs ...LedgerConfig) *InMemoryLedger {
	var cfg LedgerConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 24 * time.Hour
	}
	if cfg.CacheMaxSize <= 0 {
		cfg.CacheMaxSize = 500_000
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &InMemoryLedger{
		snapshots:   make(map[string]PositionSnapshot),
		appliedFill: ttlcache.NewNamed[struct{}]("position_applied_fill", cfg.CacheTTL, cfg.CacheMaxSize),
		logger:      logger,
	}
}

func (l *InMemoryLedger) ApplyFill(_ context.Context, fill TradeFillEvent) (PositionSnapshot, error) {
	if err := validateFill(fill); err != nil {
		return PositionSnapshot{}, err
	}

	key := positionKey(fill.AccountID, fill.Symbol)

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.appliedFill.Get(fill.TradeID); ok {
		existing, found := l.snapshots[key]
		if found {
			return existing, nil
		}
		return PositionSnapshot{AccountID: fill.AccountID, Symbol: fill.Symbol}, nil
	}

	current := l.snapshots[key]
	if current.AccountID == "" {
		current.AccountID = fill.AccountID
		current.Symbol = normalizeSymbol(fill.Symbol)
	}

	side := strings.ToLower(strings.TrimSpace(fill.Side))
	switch side {
	case "buy":
		current = applyBuy(current, fill)
	case "sell":
		var err error
		current, err = applySell(current, fill)
		if err != nil {
			return PositionSnapshot{}, err
		}
	default:
		return PositionSnapshot{}, fmt.Errorf("%w: invalid side", ErrInvalidFill)
	}

	current.UpdatedMS = time.Now().UnixMilli()
	l.snapshots[key] = current
	l.appliedFill.Set(fill.TradeID, struct{}{})

	l.logger.Info("position fill applied",
		"account_id", fill.AccountID,
		"symbol", fill.Symbol,
		"side", fill.Side,
		"fill_qty", fill.FillQty,
	)
	return current, nil
}

func (l *InMemoryLedger) Get(accountID, symbol string) (PositionSnapshot, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	snapshot, ok := l.snapshots[positionKey(accountID, symbol)]
	return snapshot, ok
}

func applyBuy(current PositionSnapshot, fill TradeFillEvent) PositionSnapshot {
	totalCostBefore := current.AvgCost * current.Quantity
	totalCostAfter := totalCostBefore + (fill.FillPrice * fill.FillQty) + fill.Fee
	newQty := current.Quantity + fill.FillQty
	current.Quantity = newQty
	if newQty > 0 {
		current.AvgCost = totalCostAfter / newQty
	}
	return current
}

func applySell(current PositionSnapshot, fill TradeFillEvent) (PositionSnapshot, error) {
	if fill.FillQty > current.Quantity {
		return PositionSnapshot{}, ErrInsufficientPosition
	}
	current.RealizedPnL += (fill.FillPrice-current.AvgCost)*fill.FillQty - fill.Fee
	current.Quantity -= fill.FillQty
	if current.Quantity == 0 {
		current.AvgCost = 0
	}
	return current, nil
}

func validateFill(fill TradeFillEvent) error {
	if strings.TrimSpace(fill.TradeID) == "" {
		return fmt.Errorf("%w: trade_id", ErrInvalidFill)
	}
	if strings.TrimSpace(fill.AccountID) == "" {
		return fmt.Errorf("%w: account_id", ErrInvalidFill)
	}
	if strings.TrimSpace(fill.Symbol) == "" {
		return fmt.Errorf("%w: symbol", ErrInvalidFill)
	}
	if fill.FillQty <= 0 {
		return fmt.Errorf("%w: qty", ErrInvalidFill)
	}
	if fill.FillPrice <= 0 {
		return fmt.Errorf("%w: price", ErrInvalidFill)
	}
	return nil
}

func positionKey(accountID, symbol string) string {
	return strings.TrimSpace(accountID) + "|" + normalizeSymbol(symbol)
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
