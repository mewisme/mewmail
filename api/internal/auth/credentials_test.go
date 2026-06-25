package auth_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"mewmail/api/internal/auth"
)

func TestLoadOrCreateAPIKey_GenerateAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")

	key1, generated, err := auth.LoadOrCreateAPIKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("expected generated=true on first run")
	}
	if len(key1) != 64 {
		t.Fatalf("expected 64-char hex key, got len %d", len(key1))
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
		}
	}

	key2, generated2, err := auth.LoadOrCreateAPIKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if generated2 {
		t.Fatal("expected generated=false on second run")
	}
	if key1 != key2 {
		t.Fatal("keys should match")
	}
}

func TestLoadOrCreateAPIKey_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")
	if err := os.WriteFile(path, []byte(`{bad`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := auth.LoadOrCreateAPIKey(path)
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestBearerAuth(t *testing.T) {
	const apiKey = "test-api-key-secret"
	log := testLogger()
	handler := auth.BearerAuth(apiKey, log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("missing auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("valid key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
	})
}
