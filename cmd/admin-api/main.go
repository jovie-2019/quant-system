package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quant-system/internal/adminapi"
	"quant-system/internal/adminstore"
	"quant-system/internal/crypto"
	"quant-system/internal/obs/logging"
	"quant-system/internal/store/mysqlstore"
)

func main() {
	logging.Init()

	addr := getenv("ADMIN_API_ADDR", ":8090")

	// --- AES Encryptor ---
	aesKey := os.Getenv("AES_KEY")
	if aesKey == "" {
		slog.Error("AES_KEY is required")
		os.Exit(1)
	}
	encryptor, err := crypto.NewEncryptor(aesKey)
	if err != nil {
		slog.Error("invalid AES_KEY", "error", err)
		os.Exit(1)
	}

	// --- MySQL ---
	mysqlDSN := os.Getenv("MYSQL_DSN")
	if mysqlDSN == "" {
		slog.Error("MYSQL_DSN is required for admin-api")
		os.Exit(1)
	}
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

	store, err := adminstore.NewStore(db)
	if err != nil {
		slog.Error("admin store init failed", "error", err)
		os.Exit(1)
	}
	if err := store.EnsureSchema(context.Background()); err != nil {
		slog.Error("admin schema migration failed", "error", err)
		os.Exit(1)
	}

	// --- Admin API Server ---
	jwtSecret := os.Getenv("JWT_SECRET")
	passHash := os.Getenv("ADMIN_PASSWORD_HASH")
	if jwtSecret == "" || passHash == "" {
		slog.Error("JWT_SECRET and ADMIN_PASSWORD_HASH are required")
		os.Exit(1)
	}

	staticDir := getenv("STATIC_DIR", "./web/dist")

	apiServer, err := adminapi.NewServer(adminapi.Config{
		Store:            store,
		Repo:             repo,
		Encryptor:        encryptor,
		JWTSecret:        jwtSecret,
		PassHash:         passHash,
		StaticDir:        staticDir,
		FeishuWebhookURL: os.Getenv("FEISHU_WEBHOOK_URL"),
	})
	if err != nil {
		slog.Error("admin api server init failed", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("admin-api listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("admin-api http failed", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("admin-api shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("admin-api shutdown failed", "error", err)
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
