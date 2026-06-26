package router

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mewmail/api/internal/config"
	"mewmail/api/internal/database"
)

func TestAPIRoutes(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := &config.Config{RequestTimeoutSec: 30, MaxBodyBytes: 1 << 20}
	h := New(Deps{Config: cfg, DB: db, Log: log, APIKey: "ext", InternalAPIKey: "int"})

	t.Run("health under api", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/health", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("health not at root", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status %d, want 404", rr.Code)
		}
	})

	t.Run("ui at root", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "MewMail") {
			t.Fatal("missing UI body")
		}
	})

	t.Run("preview without auth", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/preview/1", nil))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", rr.Code)
		}
	})

	t.Run("preview with bearer", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/preview/1", nil)
		req.Header.Set("Authorization", "Bearer ext")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code == http.StatusUnauthorized {
			t.Fatal("bearer auth rejected")
		}
	})
}
