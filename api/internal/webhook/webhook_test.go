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
	c := New(srv.URL+"/api/webhooks/1/token", "https://mail.example.com", log)
	c.EmailReceived(7, "a@x.com", "b@x.com", "hello", "<id@x.com>", "one-time-token")

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
	embed := payload.Embeds[0]
	if embed.Title != "hello" {
		t.Fatalf("title %q", embed.Title)
	}
	if embed.Description != "**a@x.com** → **b@x.com**" {
		t.Fatalf("description %q", embed.Description)
	}
	if embed.Footer == nil || embed.Footer.Text != "MewMail · New email" {
		t.Fatalf("footer %+v", embed.Footer)
	}

	var id, actions string
	for _, f := range embed.Fields {
		switch f.Name {
		case "ID":
			id = f.Value
		case "Actions":
			actions = f.Value
		}
	}
	if id != "`#7`" {
		t.Fatalf("id field %q", id)
	}
	wantActions := "[Open](https://mail.example.com/emails/preview/7?otk=one-time-token) · [Keep](https://mail.example.com/emails/7/keep?otk=one-time-token)"
	if actions != wantActions {
		t.Fatalf("actions %q", actions)
	}
}

func TestEmailOpened_GenericPayload(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Store(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c := New(srv.URL, "", log)
	openedAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	c.EmailOpened(9, "a@x.com", "b@x.com", "hi", "<id@x.com>", "preview", openedAt)

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
	if payload["event"] != "email.opened" {
		t.Fatalf("event %v", payload["event"])
	}
	data, _ := payload["data"].(map[string]any)
	if data["id"].(float64) != 9 || data["via"] != "preview" {
		t.Fatalf("data %v", data)
	}
	if data["opened_at"] != openedAt.Format(time.RFC3339) {
		t.Fatalf("opened_at %v", data["opened_at"])
	}
}

func TestEmailOpened_DiscordPayload(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Store(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c := New(srv.URL+"/api/webhooks/1/token", "", log)
	openedAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	c.EmailOpened(9, "a@x.com", "b@x.com", "hi", "<id@x.com>", "preview", openedAt)

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
	embed := payload.Embeds[0]
	if embed.Title != "hi" {
		t.Fatalf("title %q", embed.Title)
	}
	if embed.Description != "First opened via **Preview link**" {
		t.Fatalf("description %q", embed.Description)
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
	c := New(srv.URL, "", log)
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.EmailsCleaned(3, cutoff, 168)

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
	if New("", "", log).Enabled() {
		t.Fatal("empty url should be disabled")
	}
	if !New("http://example.com/hook", "", log).Enabled() {
		t.Fatal("url should enable client")
	}
}

func TestEmailReceived_PreviewURL(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Store(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c := New(srv.URL, "https://mail.example.com", log)
	c.EmailReceived(7, "a@x.com", "b@x.com", "hi", "<id@x.com>", "one-time-token")

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
	data, _ := payload["data"].(map[string]any)
	wantPreview := "https://mail.example.com/emails/preview/7?otk=one-time-token"
	if data["preview_url"] != wantPreview {
		t.Fatalf("preview_url %v, want %q", data["preview_url"], wantPreview)
	}
	wantKeep := "https://mail.example.com/emails/7/keep?otk=one-time-token"
	if data["keep_url"] != wantKeep {
		t.Fatalf("keep_url %v, want %q", data["keep_url"], wantKeep)
	}
}
