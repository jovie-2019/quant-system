package risk

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"quant-system/internal/obs/metrics"
	"quant-system/internal/obs/ttlcache"
	"quant-system/pkg/contracts"
)

type Decision = contracts.RiskDecisionType

const (
	DecisionAllow  Decision = contracts.RiskDecisionAllow
	DecisionReject Decision = contracts.RiskDecisionReject
)

type Config struct {
	MaxOrderQty    float64
	MaxOrderAmount float64
	AllowedSymbols map[string]struct{}
	CacheTTL       time.Duration
	CacheMaxSize   int
	Logger         *slog.Logger
}

type RiskDecision = contracts.RiskDecision

type RiskEngine interface {
	Evaluate(ctx context.Context, intent contracts.OrderIntent) RiskDecision
}

type InMemoryEngine struct {
	mu      sync.RWMutex
	config  Config
	decided *ttlcache.Cache[RiskDecision]
	logger  *slog.Logger
}

func NewInMemoryEngine(config Config) *InMemoryEngine {
	if config.CacheTTL <= 0 {
		config.CacheTTL = time.Hour
	}
	if config.CacheMaxSize <= 0 {
		config.CacheMaxSize = 100_000
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &InMemoryEngine{
		config:  sanitizeConfig(config),
		decided: ttlcache.NewNamed[RiskDecision]("risk_decision", config.CacheTTL, config.CacheMaxSize),
		logger:  logger,
	}
}

func (e *InMemoryEngine) SetConfig(config Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config = sanitizeConfig(config)
}

func (e *InMemoryEngine) Evaluate(_ context.Context, intent contracts.OrderIntent) RiskDecision {
	start := time.Now()

	if existing, ok := e.decided.Get(intent.IntentID); ok {
		metrics.ObserveRiskEvaluation(string(existing.Decision), time.Since(start))
		return existing
	}

	e.mu.RLock()
	config := e.config
	e.mu.RUnlock()

	decision := evaluateIntent(intent, config)

	// Double-check for race.
	if existing, ok := e.decided.Get(intent.IntentID); ok {
		metrics.ObserveRiskEvaluation(string(existing.Decision), time.Since(start))
		return existing
	}
	e.decided.Set(intent.IntentID, decision)

	e.logger.Info("risk decision",
		"intent_id", intent.IntentID,
		"symbol", intent.Symbol,
		"rule_id", decision.RuleID,
		"decision", decision.Decision,
	)
	metrics.ObserveRiskEvaluation(string(decision.Decision), time.Since(start))
	return decision
}

func evaluateIntent(intent contracts.OrderIntent, config Config) RiskDecision {
	now := time.Now().UnixMilli()

	if strings.TrimSpace(intent.IntentID) == "" {
		return reject(intent, "risk.intent_id.required", "missing_intent_id", now)
	}
	if strings.TrimSpace(intent.Symbol) == "" {
		return reject(intent, "risk.symbol.required", "missing_symbol", now)
	}
	if intent.Price <= 0 {
		return reject(intent, "risk.price.invalid", "invalid_price", now)
	}
	if intent.Quantity <= 0 {
		return reject(intent, "risk.qty.invalid", "invalid_qty", now)
	}
	if config.MaxOrderQty > 0 && intent.Quantity > config.MaxOrderQty {
		return reject(intent, "risk.qty.max", "qty_exceeds_limit", now)
	}
	if config.MaxOrderAmount > 0 && intent.Price*intent.Quantity > config.MaxOrderAmount {
		return reject(intent, "risk.amount.max", "amount_exceeds_limit", now)
	}
	if len(config.AllowedSymbols) > 0 {
		if _, ok := config.AllowedSymbols[normalizeSymbol(intent.Symbol)]; !ok {
			return reject(intent, "risk.symbol.not_allowed", "symbol_not_allowed", now)
		}
	}
	return RiskDecision{
		Intent:      intent,
		Decision:    DecisionAllow,
		RuleID:      "risk.pass",
		ReasonCode:  "ok",
		EvaluatedMS: now,
	}
}

func reject(intent contracts.OrderIntent, ruleID, reason string, ts int64) RiskDecision {
	return RiskDecision{
		Intent:      intent,
		Decision:    DecisionReject,
		RuleID:      ruleID,
		ReasonCode:  reason,
		EvaluatedMS: ts,
	}
}

func sanitizeConfig(config Config) Config {
	out := Config{
		MaxOrderQty:    config.MaxOrderQty,
		MaxOrderAmount: config.MaxOrderAmount,
		AllowedSymbols: make(map[string]struct{}, len(config.AllowedSymbols)),
		CacheTTL:       config.CacheTTL,
		CacheMaxSize:   config.CacheMaxSize,
	}
	for symbol := range config.AllowedSymbols {
		out.AllowedSymbols[normalizeSymbol(symbol)] = struct{}{}
	}
	return out
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
