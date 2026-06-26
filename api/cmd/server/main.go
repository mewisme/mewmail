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
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(runHealthcheck())
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config error", "error", err)
		os.Exit(1)
	}

	creds, changes, err := auth.LoadOrCreateCredentials(cfg.CredentialsPath)
	if err != nil {
		log.Error("credentials error", "error", err)
		os.Exit(1)
	}
	if changes.CreatedExternal && changes.CreatedInternal {
		fmt.Fprintf(os.Stdout, "\n=== MewMailAPI: credentials generated (save these, shown once) ===\nExternal API key: %s\nInternal ingest key: %s\n============================================================\n\n", creds.ExternalAPIKey, creds.InternalAPIKey)
	} else if changes.CreatedInternal {
		fmt.Fprintf(os.Stdout, "\n=== MewMailAPI: internal ingest key generated (shown once) ===\n%s\n============================================================\n\n", creds.InternalAPIKey)
	}

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Error("database error", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	wh := webhook.New(cfg.WebhookURL, cfg.PublicURL, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cl := cleaner.New(db, log, cfg.EmailRetentionHours, wh)
	go cl.Run(ctx)

	addr := fmt.Sprintf("%s:%s", cfg.APIHost, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler: router.New(router.Deps{Config: cfg, DB: db, Log: log, APIKey: creds.ExternalAPIKey, InternalAPIKey: creds.InternalAPIKey, Webhook: wh}),
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

func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/api/health/ready")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
