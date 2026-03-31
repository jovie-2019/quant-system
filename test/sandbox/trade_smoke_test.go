package sandbox

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"quant-system/internal/adapter"
	"quant-system/internal/execution"
	"quant-system/internal/orderfsm"
	"quant-system/internal/risk"
	"quant-system/pkg/contracts"
)

func TestSandboxTradeSmoke(t *testing.T) {
	if os.Getenv("RUN_SANDBOX_TESTS") != "1" {
		t.Skip("set RUN_SANDBOX_TESTS=1 to enable sandbox probes")
	}
	if os.Getenv("RUN_SANDBOX_TRADE_TESTS") != "1" {
		t.Skip("set RUN_SANDBOX_TRADE_TESTS=1 to enable trade smoke tests")
	}

	venue := strings.ToLower(envOrDefault("SANDBOX_TRADE_VENUE", "binance"))
	symbol := envOrDefault("SANDBOX_TRADE_SYMBOL", "BTC-USDT")
	side := strings.ToLower(envOrDefault("SANDBOX_TRADE_SIDE", "buy"))
	price, ok := envFloat("SANDBOX_TRADE_PRICE")
	if !ok || price <= 0 {
		t.Skip("set SANDBOX_TRADE_PRICE to a valid positive number")
	}
	qty, ok := envFloat("SANDBOX_TRADE_QTY")
	if !ok || qty <= 0 {
		t.Skip("set SANDBOX_TRADE_QTY to a valid positive number")
	}

	gateway, err := newSandboxTradeGatewayFromEnv(venue)
	if err != nil {
		t.Skipf("skip trade smoke: %v", err)
	}

	exec, err := execution.NewInMemoryExecutor(gateway)
	if err != nil {
		t.Fatalf("NewInMemoryExecutor() error = %v", err)
	}
	fsm := orderfsm.NewInMemoryStateMachine()
	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty: qty * 10,
		AllowedSymbols: map[string]struct{}{
			strings.ToUpper(strings.TrimSpace(symbol)): {},
		},
	})

	intent := contracts.OrderIntent{
		IntentID:    fmt.Sprintf("sandbox-%d", time.Now().UnixNano()),
		StrategyID:  "sandbox-smoke",
		Symbol:      symbol,
		Side:        side,
		Price:       price,
		Quantity:    qty,
		TimeInForce: "GTC",
	}
	decision := riskEngine.Evaluate(context.Background(), intent)
	if decision.Decision != risk.DecisionAllow {
		t.Fatalf("risk rejected intent: rule=%s reason=%s", decision.RuleID, decision.ReasonCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), envDuration("SANDBOX_TRADE_TIMEOUT", 20*time.Second))
	defer cancel()

	submit, err := exec.Submit(ctx, decision)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if strings.TrimSpace(submit.ClientOrderID) == "" {
		t.Fatal("empty client order id in submit ack")
	}

	order, err := fsm.Apply(orderfsm.Event{
		ClientOrderID: submit.ClientOrderID,
		VenueOrderID:  submit.VenueOrderID,
		Symbol:        symbol,
		State:         orderfsm.StateAck,
		FilledQty:     0,
		AvgPrice:      0,
	})
	if err != nil {
		t.Fatalf("fsm ack apply error = %v", err)
	}
	if order.State != orderfsm.StateAck {
		t.Fatalf("unexpected order state after ack: %s", order.State)
	}

	cancelResult, err := exec.Cancel(ctx, execution.CancelIntent{
		ClientOrderID: submit.ClientOrderID,
		VenueOrderID:  submit.VenueOrderID,
		Symbol:        symbol,
	})
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	order, err = fsm.Apply(orderfsm.Event{
		ClientOrderID: cancelResult.ClientOrderID,
		VenueOrderID:  cancelResult.VenueOrderID,
		Symbol:        symbol,
		State:         orderfsm.StateCanceled,
		FilledQty:     0,
		AvgPrice:      0,
	})
	if err != nil {
		t.Fatalf("fsm cancel apply error = %v", err)
	}
	if order.State != orderfsm.StateCanceled {
		t.Fatalf("unexpected final order state: %s", order.State)
	}

	t.Logf(
		"trade smoke ok venue=%s symbol=%s client_order_id=%s venue_order_id=%s",
		venue, symbol, submit.ClientOrderID, submit.VenueOrderID,
	)
}

func newSandboxTradeGatewayFromEnv(venue string) (adapter.TradeGateway, error) {
	switch venue {
	case "binance":
		apiKey := strings.TrimSpace(os.Getenv("SANDBOX_BINANCE_API_KEY"))
		secret := strings.TrimSpace(os.Getenv("SANDBOX_BINANCE_API_SECRET"))
		if apiKey == "" || secret == "" {
			return nil, fmt.Errorf("missing SANDBOX_BINANCE_API_KEY or SANDBOX_BINANCE_API_SECRET")
		}
		return adapter.NewBinanceSpotTradeGateway(adapter.BinanceSpotRESTConfig{
			BaseURL:      envOrDefault("SANDBOX_BINANCE_REST", "https://testnet.binance.vision"),
			APIKey:       apiKey,
			APISecret:    secret,
			RecvWindowMS: 5000,
			MinInterval:  envDuration("SANDBOX_TRADE_MIN_INTERVAL", 50*time.Millisecond),
		}, nil)
	case "okx":
		apiKey := strings.TrimSpace(os.Getenv("SANDBOX_OKX_API_KEY"))
		secret := strings.TrimSpace(os.Getenv("SANDBOX_OKX_API_SECRET"))
		passphrase := strings.TrimSpace(os.Getenv("SANDBOX_OKX_PASSPHRASE"))
		if apiKey == "" || secret == "" || passphrase == "" {
			return nil, fmt.Errorf("missing SANDBOX_OKX_API_KEY / SANDBOX_OKX_API_SECRET / SANDBOX_OKX_PASSPHRASE")
		}
		return adapter.NewOKXSpotTradeGateway(adapter.OKXSpotRESTConfig{
			BaseURL:          envOrDefault("SANDBOX_OKX_REST", "https://www.okx.com"),
			APIKey:           apiKey,
			APISecret:        secret,
			Passphrase:       passphrase,
			MinInterval:      envDuration("SANDBOX_TRADE_MIN_INTERVAL", 50*time.Millisecond),
			SimulatedTrading: envBool("SANDBOX_OKX_SIMULATED", true),
		}, nil)
	default:
		return nil, fmt.Errorf("unsupported SANDBOX_TRADE_VENUE=%s", venue)
	}
}

func envFloat(name string) (float64, bool) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
