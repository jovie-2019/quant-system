package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"quant-system/internal/adapter"
	"quant-system/internal/obs/metrics"
	"quant-system/internal/obs/ttlcache"
	"quant-system/internal/risk"
)

var (
	ErrGatewayNil                 = errors.New("execution: trade gateway is nil")
	ErrRejectedDecision           = errors.New("execution: risk decision is not allow")
	ErrInvalidIntent              = errors.New("execution: invalid order intent")
	ErrInvalidReconcileRequest    = errors.New("execution: invalid reconcile request")
	ErrOrderQueryUnsupported      = errors.New("execution: order query is not supported by gateway")
	ErrGatewayRetryableFailure    = errors.New("execution: retryable gateway failure")
	ErrGatewayNonRetryableFailure = errors.New("execution: non-retryable gateway failure")
	ErrGatewayRetryExhausted      = errors.New("execution: gateway retry exhausted")
)

type SubmitResult struct {
	IntentID      string
	ClientOrderID string
	VenueOrderID  string
	Status        string
	IdempotentHit bool
}

type CancelIntent struct {
	ClientOrderID string
	VenueOrderID  string
	Symbol        string
}

type CancelResult struct {
	ClientOrderID string
	VenueOrderID  string
	Status        string
}

type ReconcileIntent struct {
	ClientOrderID string
	VenueOrderID  string
	Symbol        string
}

type ReconcileResult struct {
	ClientOrderID string
	VenueOrderID  string
	Symbol        string
	Status        string
	FilledQty     float64
	AvgPrice      float64
}

type ExecutorConfig struct {
	CacheTTL                time.Duration
	CacheMaxSize            int
	GatewayMaxRetries       int
	GatewayPlaceMaxRetries  int
	GatewayCancelMaxRetries int
	GatewayQueryMaxRetries  int
	GatewayBackoffBase      time.Duration
	GatewayBackoffMax       time.Duration
	GatewayBackoffJitter    float64
	Logger                  *slog.Logger
}

type Executor interface {
	Submit(ctx context.Context, decision risk.RiskDecision) (SubmitResult, error)
	Cancel(ctx context.Context, req CancelIntent) (CancelResult, error)
	Reconcile(ctx context.Context, req ReconcileIntent) (ReconcileResult, error)
}

type InMemoryExecutor struct {
	mu               sync.RWMutex
	gateway          adapter.TradeGateway
	submits          *ttlcache.Cache[SubmitResult]
	placeMaxRetries  int
	cancelMaxRetries int
	queryMaxRetries  int
	backoffBase      time.Duration
	backoffMax       time.Duration
	backoffJitter    float64
	jitterFn         func() float64
	sleepWithCtx     func(context.Context, time.Duration) error
	logger           *slog.Logger
}

func NewInMemoryExecutor(gateway adapter.TradeGateway, cfgs ...ExecutorConfig) (*InMemoryExecutor, error) {
	if gateway == nil {
		return nil, ErrGatewayNil
	}
	var cfg ExecutorConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = time.Hour
	}
	if cfg.CacheMaxSize <= 0 {
		cfg.CacheMaxSize = 100_000
	}
	if cfg.GatewayMaxRetries < 0 {
		cfg.GatewayMaxRetries = 0
	}
	if cfg.GatewayBackoffBase <= 0 {
		cfg.GatewayBackoffBase = 100 * time.Millisecond
	}
	if cfg.GatewayBackoffMax <= 0 {
		cfg.GatewayBackoffMax = 2 * time.Second
	}
	if cfg.GatewayBackoffMax < cfg.GatewayBackoffBase {
		cfg.GatewayBackoffMax = cfg.GatewayBackoffBase
	}
	if cfg.GatewayBackoffJitter < 0 {
		cfg.GatewayBackoffJitter = 0
	}
	if cfg.GatewayBackoffJitter > 0.9 {
		cfg.GatewayBackoffJitter = 0.9
	}
	if cfg.GatewayBackoffJitter == 0 {
		cfg.GatewayBackoffJitter = 0.2
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &InMemoryExecutor{
		gateway:          gateway,
		submits:          ttlcache.NewNamed[SubmitResult]("execution_submit", cfg.CacheTTL, cfg.CacheMaxSize),
		placeMaxRetries:  chooseRetryBudget(cfg.GatewayPlaceMaxRetries, cfg.GatewayMaxRetries),
		cancelMaxRetries: chooseRetryBudget(cfg.GatewayCancelMaxRetries, cfg.GatewayMaxRetries),
		queryMaxRetries:  chooseRetryBudget(cfg.GatewayQueryMaxRetries, cfg.GatewayMaxRetries),
		backoffBase:      cfg.GatewayBackoffBase,
		backoffMax:       cfg.GatewayBackoffMax,
		backoffJitter:    cfg.GatewayBackoffJitter,
		jitterFn:         rand.Float64,
		sleepWithCtx:     sleepWithContext,
		logger:           logger,
	}, nil
}

func (e *InMemoryExecutor) Submit(ctx context.Context, decision risk.RiskDecision) (SubmitResult, error) {
	start := time.Now()
	outcome := "success"
	defer func() {
		metrics.ObserveExecutionSubmit(outcome, time.Since(start))
	}()

	if decision.Decision != risk.DecisionAllow {
		outcome = "rejected"
		return SubmitResult{}, fmt.Errorf("%w: %s", ErrRejectedDecision, decision.Decision)
	}
	intent := decision.Intent
	if strings.TrimSpace(intent.IntentID) == "" || strings.TrimSpace(intent.Symbol) == "" || intent.Quantity <= 0 || intent.Price <= 0 {
		outcome = "error"
		return SubmitResult{}, ErrInvalidIntent
	}

	if existing, ok := e.submits.Get(intent.IntentID); ok {
		existing.IdempotentHit = true
		return existing, nil
	}

	clientOrderID := clientOrderIDFromIntent(intent.IntentID)
	ack, err := e.placeOrderWithRetry(ctx, adapter.VenueOrderRequest{
		ClientOrderID: clientOrderID,
		Symbol:        intent.Symbol,
		Side:          intent.Side,
		Price:         intent.Price,
		Quantity:      intent.Quantity,
	})
	if err != nil {
		outcome = "error"
		return SubmitResult{}, classifyGatewayError(err)
	}

	result := SubmitResult{
		IntentID:      intent.IntentID,
		ClientOrderID: ack.ClientOrderID,
		VenueOrderID:  ack.VenueOrderID,
		Status:        ack.Status,
		IdempotentHit: false,
	}

	if existing, ok := e.submits.Get(intent.IntentID); ok {
		existing.IdempotentHit = true
		return existing, nil
	}
	e.submits.Set(intent.IntentID, result)

	e.logger.Info("execution submit",
		"intent_id", intent.IntentID,
		"client_order_id", clientOrderID,
		"outcome", outcome,
	)
	return result, nil
}

func (e *InMemoryExecutor) Cancel(ctx context.Context, req CancelIntent) (CancelResult, error) {
	ack, err := e.cancelOrderWithRetry(ctx, adapter.VenueCancelRequest{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  req.VenueOrderID,
		Symbol:        req.Symbol,
	})
	if err != nil {
		return CancelResult{}, classifyGatewayError(err)
	}
	return CancelResult{
		ClientOrderID: ack.ClientOrderID,
		VenueOrderID:  ack.VenueOrderID,
		Status:        ack.Status,
	}, nil
}

func (e *InMemoryExecutor) Reconcile(ctx context.Context, req ReconcileIntent) (ReconcileResult, error) {
	if strings.TrimSpace(req.Symbol) == "" || (strings.TrimSpace(req.ClientOrderID) == "" && strings.TrimSpace(req.VenueOrderID) == "") {
		return ReconcileResult{}, ErrInvalidReconcileRequest
	}

	queryGateway, ok := e.gateway.(adapter.OrderQueryGateway)
	if !ok {
		return ReconcileResult{}, ErrOrderQueryUnsupported
	}

	status, err := e.queryOrderWithRetry(ctx, queryGateway, adapter.VenueOrderQueryRequest{
		ClientOrderID: req.ClientOrderID,
		VenueOrderID:  req.VenueOrderID,
		Symbol:        req.Symbol,
	})
	if err != nil {
		return ReconcileResult{}, classifyGatewayError(err)
	}

	return ReconcileResult{
		ClientOrderID: status.ClientOrderID,
		VenueOrderID:  status.VenueOrderID,
		Symbol:        status.Symbol,
		Status:        status.Status,
		FilledQty:     status.FilledQty,
		AvgPrice:      status.AvgPrice,
	}, nil
}

func (e *InMemoryExecutor) placeOrderWithRetry(ctx context.Context, req adapter.VenueOrderRequest) (adapter.VenueOrderAck, error) {
	var ack adapter.VenueOrderAck
	err := e.withGatewayRetry(ctx, "place_order", e.placeMaxRetries, func(ctx context.Context) error {
		var err error
		ack, err = e.gateway.PlaceOrder(ctx, req)
		return err
	})
	if err != nil {
		return adapter.VenueOrderAck{}, err
	}
	return ack, nil
}

func (e *InMemoryExecutor) cancelOrderWithRetry(ctx context.Context, req adapter.VenueCancelRequest) (adapter.VenueCancelAck, error) {
	var ack adapter.VenueCancelAck
	err := e.withGatewayRetry(ctx, "cancel_order", e.cancelMaxRetries, func(ctx context.Context) error {
		var err error
		ack, err = e.gateway.CancelOrder(ctx, req)
		return err
	})
	if err != nil {
		return adapter.VenueCancelAck{}, err
	}
	return ack, nil
}

func (e *InMemoryExecutor) queryOrderWithRetry(ctx context.Context, gateway adapter.OrderQueryGateway, req adapter.VenueOrderQueryRequest) (adapter.VenueOrderStatus, error) {
	var status adapter.VenueOrderStatus
	err := e.withGatewayRetry(ctx, "query_order", e.queryMaxRetries, func(ctx context.Context) error {
		var err error
		status, err = gateway.QueryOrder(ctx, req)
		return err
	})
	if err != nil {
		return adapter.VenueOrderStatus{}, err
	}
	return status, nil
}

func (e *InMemoryExecutor) withGatewayRetry(ctx context.Context, operation string, maxRetries int, fn func(context.Context) error) error {
	for attempt := 0; ; attempt++ {
		err := fn(ctx)
		if err == nil {
			metrics.ObserveExecutionGateway(operation, "success")
			return nil
		}
		if !errors.Is(err, adapter.ErrGatewayRetryable) {
			metrics.ObserveExecutionGateway(operation, "non_retryable_error")
			return err
		}
		if attempt >= maxRetries {
			metrics.ObserveExecutionGateway(operation, "retry_exhausted")
			return fmt.Errorf("%w: operation=%s retries=%d last=%v", ErrGatewayRetryExhausted, operation, attempt, err)
		}

		metrics.ObserveExecutionGateway(operation, "retry")
		backoff := e.retryBackoff(attempt)
		e.logger.Warn("execution gateway retry",
			"operation", operation,
			"attempt", attempt+1,
			"backoff_ms", backoff.Milliseconds(),
			"error", err,
		)
		if err := e.sleepWithCtx(ctx, backoff); err != nil {
			metrics.ObserveExecutionGateway(operation, "retry_sleep_interrupted")
			return err
		}
	}
}

func (e *InMemoryExecutor) retryBackoff(attempt int) time.Duration {
	backoff := e.backoffBase
	for i := 0; i < attempt; i++ {
		if backoff >= e.backoffMax/2 {
			return e.backoffMax
		}
		backoff *= 2
	}
	if backoff > e.backoffMax {
		return e.backoffMax
	}
	if e.backoffJitter > 0 && e.jitterFn != nil {
		// jitter in range [1-jitter, 1+jitter], clamped by max.
		factor := 1 + ((e.jitterFn()*2 - 1) * e.backoffJitter)
		if factor < 0 {
			factor = 0
		}
		jittered := time.Duration(float64(backoff) * factor)
		if jittered > e.backoffMax {
			return e.backoffMax
		}
		if jittered <= 0 {
			return time.Millisecond
		}
		return jittered
	}
	return backoff
}

func classifyGatewayError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, adapter.ErrGatewayNonRetryable):
		return errors.Join(ErrGatewayNonRetryableFailure, err)
	case errors.Is(err, adapter.ErrGatewayRetryable), errors.Is(err, ErrGatewayRetryExhausted):
		return errors.Join(ErrGatewayRetryableFailure, err)
	default:
		return err
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func chooseRetryBudget(opValue int, fallback int) int {
	if opValue == 0 {
		return fallback
	}
	if opValue > 0 {
		return opValue
	}
	return 0
}

func clientOrderIDFromIntent(intentID string) string {
	const maxLen = 32
	normalized := strings.ReplaceAll(strings.TrimSpace(intentID), " ", "-")
	if len(normalized) > maxLen {
		// Use a hash suffix to preserve uniqueness when truncating.
		h := uint64(0)
		for _, b := range []byte(normalized) {
			h = h*31 + uint64(b)
		}
		suffix := fmt.Sprintf("-%x", h&0xFFFFFFFF)
		normalized = normalized[:maxLen-len(suffix)] + suffix
	}
	return "cid-" + normalized
}
