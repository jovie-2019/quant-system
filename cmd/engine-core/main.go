package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quant-system/internal/adminstore"
	"quant-system/internal/bus/natsbus"
	"quant-system/internal/controlapi"
	"quant-system/internal/crypto"
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
	var db *sql.DB
	if mysqlDSN != "" {
		var err error
		db, err = mysqlstore.Open(mysqlstore.Config{DSN: mysqlDSN})
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
	fsm := orderfsm.NewInMemoryStateMachine()
	ledger := position.NewInMemoryLedger()

	// --- Dynamic gateway pool (DB-backed API keys) ---
	var gatewayPool *pipeline.GatewayPool
	var adminStore *adminstore.Store
	aesKey := os.Getenv("AES_KEY")
	if db != nil && aesKey != "" {
		encryptor, err := crypto.NewEncryptor(aesKey)
		if err != nil {
			slog.Error("aes encryptor init failed", "error", err)
			os.Exit(1)
		}
		adminStore, err = adminstore.NewStore(db)
		if err != nil {
			slog.Error("admin store init failed", "error", err)
			os.Exit(1)
		}
		if err := adminStore.EnsureSchema(context.Background()); err != nil {
			slog.Error("admin store schema migration failed", "error", err)
			os.Exit(1)
		}
		gatewayPool = pipeline.NewGatewayPool(adminStore, encryptor, nil)
		slog.Info("dynamic gateway pool initialized")
	} else {
		slog.Warn("AES_KEY or MYSQL_DSN not set, dynamic gateway pool disabled")
	}

	// --- Pipeline ---
	pipe, err := pipeline.New(client, riskEngine, nil, fsm, ledger, persister, pipeline.Config{
		AccountID:     accountID,
		DeliverPolicy: deliverPolicy,
		SimulateFill:  simulateFill,
		GatewayPool:   gatewayPool,
		AdminStore:    adminStore,
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

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
