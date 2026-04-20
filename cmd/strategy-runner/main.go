package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"quant-system/internal/adminstore"
	"quant-system/internal/bus/natsbus"
	"quant-system/internal/obs/logging"
	"quant-system/internal/obs/metrics"
	"quant-system/internal/store/mysqlstore"
	"quant-system/internal/strategy"
	"quant-system/internal/strategyrunner"

	// Blank-import strategy packages to trigger init() registration.
	_ "quant-system/internal/strategy/momentum"
)

func main() {
	logging.Init()

	// -----------------------------------------------------------------------
	// Required env vars
	// -----------------------------------------------------------------------
	configIDStr := os.Getenv("STRATEGY_CONFIG_ID")
	if configIDStr == "" {
		slog.Error("STRATEGY_CONFIG_ID is required")
		os.Exit(1)
	}
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		slog.Error("STRATEGY_CONFIG_ID must be an integer", "value", configIDStr, "error", err)
		os.Exit(1)
	}

	mysqlDSN := os.Getenv("MYSQL_DSN")
	if mysqlDSN == "" {
		slog.Error("MYSQL_DSN is required")
		os.Exit(1)
	}

	natsURL := getenv("NATS_URL", "nats://nats:4222")
	runnerAddr := getenv("STRATEGY_RUNNER_ADDR", ":8081")

	// -----------------------------------------------------------------------
	// MySQL
	// -----------------------------------------------------------------------
	db, err := mysqlstore.Open(mysqlstore.Config{DSN: mysqlDSN})
	if err != nil {
		slog.Error("mysql open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	store, err := adminstore.NewStore(db)
	if err != nil {
		slog.Error("adminstore init failed", "error", err)
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Load initial strategy config from DB
	// -----------------------------------------------------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, found, err := store.GetStrategyConfig(ctx, configID)
	if err != nil {
		slog.Error("load strategy config failed", "config_id", configID, "error", err)
		os.Exit(1)
	}
	if !found {
		slog.Error("strategy config not found", "config_id", configID)
		os.Exit(1)
	}
	if cfg.Status == "stopped" {
		slog.Info("strategy config is stopped, exiting", "config_id", configID)
		os.Exit(0)
	}

	slog.Info("loaded strategy config",
		"config_id", cfg.ID,
		"strategy_id", cfg.StrategyID,
		"strategy_type", cfg.StrategyType,
		"status", cfg.Status,
	)

	// -----------------------------------------------------------------------
	// NATS
	// -----------------------------------------------------------------------
	client, err := natsbus.Connect(natsbus.Config{
		URL:  natsURL,
		Name: fmt.Sprintf("strategy-runner-%d", configID),
	})
	if err != nil {
		slog.Error("nats connect failed", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := ensureStreams(ctx, client); err != nil {
		slog.Error("ensure stream failed", "error", err)
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Strategy runtime
	//
	// Build order matters: the IntentSink is gated by the control handler,
	// so we construct the handler first (which needs a strategy instance)
	// and then wrap the live sink with pause/shadow routing.
	// -----------------------------------------------------------------------
	currentStrategy, err := createStrategy(cfg)
	if err != nil {
		slog.Error("create strategy failed", "error", err)
		os.Exit(1)
	}

	controlHandler, err := strategyrunner.NewControlHandler(client, currentStrategy, strategyrunner.ControlConfig{
		StrategyID: cfg.StrategyID,
	})
	if err != nil {
		slog.Error("control handler init failed", "error", err)
		os.Exit(1)
	}

	liveIntentSink, err := strategyrunner.NewNATSIntentSink(client)
	if err != nil {
		slog.Error("intent sink init failed", "error", err)
		os.Exit(1)
	}
	intentSink := strategyrunner.GatedIntentSink(controlHandler, liveIntentSink, client)

	runtime, err := strategy.NewInMemoryRuntime(intentSink)
	if err != nil {
		slog.Error("runtime init failed", "error", err)
		os.Exit(1)
	}

	if err := runtime.Register(currentStrategy); err != nil {
		slog.Error("register strategy failed", "error", err)
		os.Exit(1)
	}

	slog.Info("strategy registered",
		"strategy_id", currentStrategy.ID(),
		"strategy_type", cfg.StrategyType,
	)

	// Subscribe to the strategy's control subject so admin-api (or an
	// optimiser pipeline) can hot-reload params / pause / shadow-mode
	// without restarting this process.
	ctlSub, err := controlHandler.Start(ctx)
	if err != nil {
		slog.Error("control subscribe failed", "error", err)
		os.Exit(1)
	}
	defer ctlSub.Unsubscribe()
	slog.Info("control channel subscribed",
		"subject", natsbus.SubjectStrategyControl(cfg.StrategyID))

	// -----------------------------------------------------------------------
	// NATS subscription loop (market events -> strategy)
	// -----------------------------------------------------------------------
	durable := fmt.Sprintf("strategy-runner-%d", configID)
	subject := getenv("STRATEGY_RUNNER_SUBJECT", "market.normalized.spot.>")
	deliverPolicy := getenv("NATS_DELIVER_POLICY", "all")

	loop, err := strategyrunner.NewLoop(client, runtime, strategyrunner.Config{
		Subject:       subject,
		Durable:       durable,
		Queue:         fmt.Sprintf("strategy-runner-%d", configID),
		DeliverPolicy: deliverPolicy,
	})
	if err != nil {
		slog.Error("loop init failed", "error", err)
		os.Exit(1)
	}

	sub, err := loop.Start(ctx)
	if err != nil {
		slog.Error("subscribe failed", "error", err)
		os.Exit(1)
	}
	defer sub.Unsubscribe()

	// -----------------------------------------------------------------------
	// HTTP health + metrics endpoint
	// -----------------------------------------------------------------------
	server := &http.Server{
		Addr:              runnerAddr,
		Handler:           newHTTPHandler(configID, db),
		ReadHeaderTimeout: 2 * time.Second,
	}

	go func() {
		slog.Info("strategy-runner listening", "addr", runnerAddr, "config_id", configID)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	// -----------------------------------------------------------------------
	// Config poll loop + signal handling
	// -----------------------------------------------------------------------
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	currentConfigHash := hashString(cfg.ConfigJSON)

	pollTicker := time.NewTicker(5 * time.Second)
	defer pollTicker.Stop()

	for {
		select {
		case <-sigCh:
			slog.Info("received shutdown signal", "config_id", configID)
			goto shutdown

		case <-ctx.Done():
			goto shutdown

		case <-pollTicker.C:
			newCfg, found, err := store.GetStrategyConfig(ctx, configID)
			if err != nil {
				slog.Warn("config poll failed", "config_id", configID, "error", err)
				continue
			}
			if !found {
				slog.Warn("strategy config disappeared from DB, shutting down", "config_id", configID)
				goto shutdown
			}

			// Check for stop signal.
			if newCfg.Status == "stopped" {
				slog.Info("strategy status changed to stopped, shutting down",
					"config_id", configID,
					"strategy_id", newCfg.StrategyID,
				)
				goto shutdown
			}

			// Check for config change (hot-reload).
			newHash := hashString(newCfg.ConfigJSON)
			if newHash != currentConfigHash {
				slog.Info("config_json changed, hot-reloading strategy",
					"config_id", configID,
					"strategy_type", newCfg.StrategyType,
					"old_hash", currentConfigHash,
					"new_hash", newHash,
				)

				newStrategy, err := createStrategy(newCfg)
				if err != nil {
					slog.Error("hot-reload: create strategy failed, keeping old strategy",
						"error", err,
					)
					continue
				}

				if err := runtime.Replace(currentStrategy, newStrategy); err != nil {
					slog.Error("hot-reload: replace strategy failed",
						"error", err,
					)
					continue
				}

				currentStrategy = newStrategy
				currentConfigHash = newHash

				slog.Info("strategy hot-reloaded successfully",
					"strategy_id", currentStrategy.ID(),
					"strategy_type", newCfg.StrategyType,
				)
			}
		}
	}

shutdown:
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown failed", "error", err)
	}
	slog.Info("strategy-runner stopped", "config_id", configID)
}

// createStrategy looks up the strategy type in the registry and creates an instance.
func createStrategy(cfg adminstore.StrategyConfig) (strategy.Strategy, error) {
	ctor, ok := strategy.Lookup(cfg.StrategyType)
	if !ok {
		return nil, fmt.Errorf("unknown strategy type %q (registered: %v)",
			cfg.StrategyType, strategy.RegisteredTypes())
	}
	s, err := ctor(json.RawMessage(cfg.ConfigJSON))
	if err != nil {
		return nil, fmt.Errorf("create strategy %q: %w", cfg.StrategyType, err)
	}
	return s, nil
}

func ensureStreams(ctx context.Context, client *natsbus.Client) error {
	if err := client.EnsureStream(ctx, natsbus.StreamConfig{
		Name:     "STREAM_MARKET",
		Subjects: []string{"market.normalized.spot.>"},
	}); err != nil {
		return err
	}
	if err := client.EnsureStream(ctx, natsbus.StreamConfig{
		Name:     "STREAM_TRADING",
		Subjects: []string{"strategy.intent.>"},
	}); err != nil {
		return err
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func newHTTPHandler(configID int64, db *sql.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ok",
			"config_id": configID,
			"types":     strategy.RegisteredTypes(),
		})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, metrics.ExposePrometheus())
	})
	return mux
}
