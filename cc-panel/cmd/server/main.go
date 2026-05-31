package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/cc-panel/internal/api"
	"github.com/example/cc-panel/internal/auth"
	"github.com/example/cc-panel/internal/config"
	secretcrypto "github.com/example/cc-panel/internal/crypto"
	"github.com/example/cc-panel/internal/db"
	"github.com/example/cc-panel/internal/scheduler"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	box, err := secretcrypto.NewSecretBox(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("create secret box: %v", err)
	}

	authService := auth.NewService(pool, cfg.JWTSecret, cfg.TokenTTL)
	if err := authService.EnsureAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}

	router := api.NewApp(api.Dependencies{
		DB:          pool,
		Config:      cfg,
		SecretBox:   box,
		AuthService: authService,
	})

	scheduler.StartGeoCIDRSync(ctx, router.Geo, cfg.GeoCIDRSyncInterval)
	scheduler.StartMonitorCollect(ctx, router.Monitor, router.Policy, cfg.MonitorCollectInterval)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router.Handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("cc-panel API listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
