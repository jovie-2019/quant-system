package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"quant-system/internal/adapter"
	"quant-system/internal/adminstore"
	"quant-system/internal/crypto"
	"quant-system/internal/execution"
)

// GatewayPool creates and caches TradeGateway and Executor instances per api_key_id.
// Gateways are resolved dynamically from the database with encrypted credentials.
type GatewayPool struct {
	store     *adminstore.Store
	encryptor *crypto.Encryptor
	mu        sync.RWMutex
	gateways  map[int64]adapter.TradeGateway
	executors map[int64]*execution.InMemoryExecutor
	logger    *slog.Logger
}

// NewGatewayPool creates a GatewayPool backed by the admin store and AES encryptor.
func NewGatewayPool(store *adminstore.Store, encryptor *crypto.Encryptor, logger *slog.Logger) *GatewayPool {
	if logger == nil {
		logger = slog.Default()
	}
	return &GatewayPool{
		store:     store,
		encryptor: encryptor,
		gateways:  make(map[int64]adapter.TradeGateway),
		executors: make(map[int64]*execution.InMemoryExecutor),
		logger:    logger,
	}
}

// Get returns a cached gateway or creates one by decrypting the API key from DB.
func (p *GatewayPool) Get(ctx context.Context, apiKeyID int64) (adapter.TradeGateway, error) {
	p.mu.RLock()
	if gw, ok := p.gateways[apiKeyID]; ok {
		p.mu.RUnlock()
		return gw, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	if gw, ok := p.gateways[apiKeyID]; ok {
		return gw, nil
	}

	// Load API key record.
	apiKey, found, err := p.store.GetAPIKey(ctx, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("gateway_pool: load api key %d: %w", apiKeyID, err)
	}
	if !found {
		return nil, fmt.Errorf("gateway_pool: api key %d not found", apiKeyID)
	}

	// Load exchange to determine venue.
	exchange, found, err := p.store.GetExchange(ctx, apiKey.ExchangeID)
	if err != nil {
		return nil, fmt.Errorf("gateway_pool: load exchange %d: %w", apiKey.ExchangeID, err)
	}
	if !found {
		return nil, fmt.Errorf("gateway_pool: exchange %d not found for api key %d", apiKey.ExchangeID, apiKeyID)
	}

	// Decrypt credentials.
	decKey, err := p.encryptor.Decrypt(apiKey.APIKeyEnc)
	if err != nil {
		return nil, fmt.Errorf("gateway_pool: decrypt api_key for %d: %w", apiKeyID, err)
	}
	decSecret, err := p.encryptor.Decrypt(apiKey.APISecretEnc)
	if err != nil {
		return nil, fmt.Errorf("gateway_pool: decrypt api_secret for %d: %w", apiKeyID, err)
	}

	var passphrase string
	if apiKey.PassphraseEnc != "" {
		passphrase, err = p.encryptor.Decrypt(apiKey.PassphraseEnc)
		if err != nil {
			return nil, fmt.Errorf("gateway_pool: decrypt passphrase for %d: %w", apiKeyID, err)
		}
	}

	venue := strings.ToLower(strings.TrimSpace(exchange.Venue))

	var gw adapter.TradeGateway
	switch venue {
	case "binance":
		gw, err = adapter.NewBinanceSpotTradeGateway(adapter.BinanceSpotRESTConfig{
			BaseURL:   "https://api.binance.com",
			APIKey:    decKey,
			APISecret: decSecret,
		}, nil)
	case "okx":
		gw, err = adapter.NewOKXSpotTradeGateway(adapter.OKXSpotRESTConfig{
			BaseURL:    "https://www.okx.com",
			APIKey:     decKey,
			APISecret:  decSecret,
			Passphrase: passphrase,
		}, nil)
	default:
		return nil, fmt.Errorf("gateway_pool: unsupported venue %q for api key %d", venue, apiKeyID)
	}
	if err != nil {
		return nil, fmt.Errorf("gateway_pool: create %s gateway for api key %d: %w", venue, apiKeyID, err)
	}

	p.gateways[apiKeyID] = gw
	p.logger.Info("gateway_pool: created gateway", "api_key_id", apiKeyID, "venue", venue)
	return gw, nil
}

// GetExecutor returns a cached executor for the given api_key_id, creating one if needed.
// Executors are cached to preserve idempotency caches across requests.
func (p *GatewayPool) GetExecutor(ctx context.Context, apiKeyID int64) (execution.Executor, error) {
	p.mu.RLock()
	if exec, ok := p.executors[apiKeyID]; ok {
		p.mu.RUnlock()
		return exec, nil
	}
	p.mu.RUnlock()

	// Get (or create) the gateway first, then create the executor under write lock.
	gw, err := p.Get(ctx, apiKeyID)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	if exec, ok := p.executors[apiKeyID]; ok {
		return exec, nil
	}

	exec, err := execution.NewInMemoryExecutor(gw)
	if err != nil {
		return nil, fmt.Errorf("gateway_pool: create executor for api key %d: %w", apiKeyID, err)
	}

	p.executors[apiKeyID] = exec
	p.logger.Info("gateway_pool: created executor", "api_key_id", apiKeyID)
	return exec, nil
}

// Invalidate removes a cached gateway and executor (e.g. when API key is updated/deleted).
func (p *GatewayPool) Invalidate(apiKeyID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.gateways, apiKeyID)
	delete(p.executors, apiKeyID)
	p.logger.Info("gateway_pool: invalidated", "api_key_id", apiKeyID)
}
