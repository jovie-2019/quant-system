package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"quant-system/internal/adapter"
	"quant-system/internal/bus/natsbus"
	"quant-system/internal/controlapi"
	"quant-system/internal/execution"
	"quant-system/internal/obs/logging"
	"quant-system/internal/orderfsm"
	"quant-system/internal/pipeline"
	"quant-system/internal/position"
	"quant-system/internal/risk"
	"quant-system/internal/store/mysqlstore"
)

func main() {
	logging.Init()

	addr := getenv("ENGINE_CORE_ADDR", ":8080")
	natsURL := getenv("NATS_URL", "nats://127.0.0.1:4222")
	mysqlDSN := os.Getenv("MYSQL_DSN")
	accountID := getenv("ACCOUNT_ID", "default")
	simulateFill := getenv("SIMULATE_FILL", "true") == "true"
	deliverPolicy := getenv("NATS_DELIVER_POLICY", "all")

	// --- NATS ---
	client, err := natsbus.Connect(natsbus.Config{
		URL:  natsURL,
		Name: "engine-core",
	})
	if err != nil {
		slog.Error("nats connect failed", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := pipeline.EnsureStreams(context.Background(), client); err != nil {
		slog.Error("ensure streams failed", "error", err)
		os.Exit(1)
	}

	// --- MySQL (optional) ---
	var persister pipeline.Persister
	if mysqlDSN != "" {
		db, err := mysqlstore.Open(mysqlstore.Config{DSN: mysqlDSN})
		if err != nil {
			slog.Error("mysql open failed", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		repo, err := mysqlstore.NewRepository(db)
		if err != nil {
			slog.Error("mysql repository init failed", "error", err)
			os.Exit(1)
		}
		if err := repo.EnsureSchema(context.Background()); err != nil {
			slog.Error("mysql schema migration failed", "error", err)
			os.Exit(1)
		}
		persister = repo
		slog.Info("mysql connected")
	} else {
		slog.Warn("MYSQL_DSN not set, running without persistence")
	}

	// --- Core components ---
	riskEngine := risk.NewInMemoryEngine(risk.Config{
		MaxOrderQty:    10,
		MaxOrderAmount: 1_000_000,
		AllowedSymbols: map[string]struct{}{
			"BTC-USDT": {},
			"ETH-USDT": {},
		},
	})

	// Gateway: select based on TRADE_VENUE.
	gateway, err := createTradeGateway()
	if err != nil {
		slog.Error("trade gateway init failed", "error", err)
		os.Exit(1)
	}

	exec, err := execution.NewInMemoryExecutor(gateway)
	if err != nil {
		slog.Error("executor init failed", "error", err)
		os.Exit(1)
	}
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()

	// --- Pipeline ---
	pipe, err := pipeline.New(client, riskEngine, exec, fsm, ledger, persister, pipeline.Config{
		AccountID:     accountID,
		DeliverPolicy: deliverPolicy,
		SimulateFill:  simulateFill,
	})
	if err != nil {
		slog.Error("pipeline init failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := pipe.Start(ctx)
	if err != nil {
		slog.Error("pipeline subscribe failed", "error", err)
		os.Exit(1)
	}
	defer sub.Unsubscribe()
	slog.Info("pipeline started, consuming strategy.intent.>")

	// --- HTTP control API ---
	server := &http.Server{
		Addr:              addr,
		Handler:           controlapi.NewServer(riskEngine),
		ReadHeaderTimeout: 2 * time.Second,
	}

	go func() {
		slog.Info("engine-core listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("engine-core listen failed", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("shutting down")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("engine-core shutdown failed", "error", err)
		os.Exit(1)
	}
}

func createTradeGateway() (adapter.TradeGateway, error) {
	venue := strings.ToLower(strings.TrimSpace(os.Getenv("TRADE_VENUE")))
	switch venue {
	case "binance":
		apiKey := os.Getenv("BINANCE_API_KEY")
		apiSecret := os.Getenv("BINANCE_API_SECRET")
		if apiKey == "" || apiSecret == "" {
			return nil, fmt.Errorf("BINANCE_API_KEY and BINANCE_API_SECRET are required for venue=binance")
		}
		gw, err := adapter.NewBinanceSpotTradeGateway(adapter.BinanceSpotRESTConfig{
			BaseURL:   getenv("BINANCE_REST_BASE_URL", "https://api.binance.com"),
			APIKey:    apiKey,
			APISecret: apiSecret,
		}, nil)
		if err != nil {
			return nil, err
		}
		slog.Info("trade gateway initialized", "venue", "binance")
		return gw, nil

	case "okx":
		apiKey := os.Getenv("OKX_API_KEY")
		apiSecret := os.Getenv("OKX_API_SECRET")
		passphrase := os.Getenv("OKX_PASSPHRASE")
		if apiKey == "" || apiSecret == "" || passphrase == "" {
			return nil, fmt.Errorf("OKX_API_KEY, OKX_API_SECRET, and OKX_PASSPHRASE are required for venue=okx")
		}
		simulated := getenv("OKX_SIMULATED_TRADING", "false") == "true"
		gw, err := adapter.NewOKXSpotTradeGateway(adapter.OKXSpotRESTConfig{
			BaseURL:          getenv("OKX_REST_BASE_URL", "https://www.okx.com"),
			APIKey:           apiKey,
			APISecret:        apiSecret,
			Passphrase:       passphrase,
			SimulatedTrading: simulated,
		}, nil)
		if err != nil {
			return nil, err
		}
		slog.Info("trade gateway initialized", "venue", "okx", "simulated", simulated)
		return gw, nil

	case "":
		slog.Warn("TRADE_VENUE not set, using stub gateway (orders will fail)")
		return adapter.StubTradeGateway{}, nil

	default:
		return nil, fmt.Errorf("unsupported TRADE_VENUE: %s (supported: binance, okx)", venue)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
