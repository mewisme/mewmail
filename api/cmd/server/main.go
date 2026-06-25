package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mewmail/api/internal/auth"
	"mewmail/api/internal/cleaner"
	"mewmail/api/internal/config"
	"mewmail/api/internal/database"
	"mewmail/api/internal/router"
	"mewmail/api/internal/webhook"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config error", "error", err)
		os.Exit(1)
	}

	apiKey, generated, err := auth.LoadOrCreateAPIKey(cfg.CredentialsPath)
	if err != nil {
		log.Error("credentials error", "error", err)
		os.Exit(1)
	}
	if generated {
		fmt.Fprintf(os.Stdout, "\n=== MewMailAPI: API key generated (save this, shown once) ===\n%s\n============================================================\n\n", apiKey)
	}

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Error("database error", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	wh := webhook.New(cfg.WebhookURL, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cl := cleaner.New(db, log, cfg.EmailRetentionHours, wh)
	go cl.Run(ctx)

	addr := fmt.Sprintf("%s:%s", cfg.APIHost, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router.New(router.Deps{Config: cfg, DB: db, Log: log, APIKey: apiKey, Webhook: wh}),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("startup", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutdown")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}
