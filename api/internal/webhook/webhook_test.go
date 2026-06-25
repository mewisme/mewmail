package webhook

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestEmailReceived_DiscordPayload(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Store(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c := New(srv.URL+"/api/webhooks/1/token", log)
	c.EmailReceived(42, "a@x.com", "b@x.com", "hello", "<id@x.com>")

	deadline := time.Now().Add(2 * time.Second)
	for got.Load() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	raw, ok := got.Load().([]byte)
	if !ok {
		t.Fatal("no webhook received")
	}

	var payload discordPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Username != "MewMail" || len(payload.Embeds) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Embeds[0].Title != "Email received" {
		t.Fatalf("title %q", payload.Embeds[0].Title)
	}
}

func TestEmailsCleaned_GenericPayload(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Store(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c := New(srv.URL, log)
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.EmailsCleaned(3, cutoff, 7)

	deadline := time.Now().Add(2 * time.Second)
	for got.Load() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	raw, ok := got.Load().([]byte)
	if !ok {
		t.Fatal("no webhook received")
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["event"] != "email.cleaned" {
		t.Fatalf("event %v", payload["event"])
	}
	data, _ := payload["data"].(map[string]any)
	if data["count"].(float64) != 3 {
		t.Fatalf("count %v", data["count"])
	}
}

func TestEnabled(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if New("", log).Enabled() {
		t.Fatal("empty url should be disabled")
	}
	if !New("http://example.com/hook", log).Enabled() {
		t.Fatal("url should enable client")
	}
}
