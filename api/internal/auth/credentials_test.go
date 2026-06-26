package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"mewmail/api/internal/auth"
	"mewmail/api/internal/models"
)

func TestLoadOrCreateCredentials_GenerateAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")

	creds1, changes, err := auth.LoadOrCreateCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if !changes.CreatedExternal || !changes.CreatedInternal {
		t.Fatalf("expected both keys created, got %+v", changes)
	}
	if len(creds1.ExternalAPIKey) != 32 || len(creds1.InternalAPIKey) != 32 {
		t.Fatalf("expected 32-char hex keys, got ext=%d int=%d", len(creds1.ExternalAPIKey), len(creds1.InternalAPIKey))
	}
	if creds1.ExternalAPIKey == creds1.InternalAPIKey {
		t.Fatal("external and internal keys should differ")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Fatalf("expected mode 0644, got %o", info.Mode().Perm())
		}
	}

	var onDisk models.CredentialsFile
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.APIKey != creds1.ExternalAPIKey || onDisk.InternalKey != creds1.InternalAPIKey {
		t.Fatal("file contents mismatch")
	}

	creds2, changes2, err := auth.LoadOrCreateCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if changes2.CreatedExternal || changes2.CreatedInternal {
		t.Fatal("expected no new keys on second run")
	}
	if creds1 != creds2 {
		t.Fatal("keys should match on reload")
	}
}

func TestLoadOrCreateCredentials_MigratesLegacyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")
	legacy := models.CredentialsFile{
		APIKey:    "legacy-external-key-0123456789ab",
		CreatedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	out, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}

	creds, changes, err := auth.LoadOrCreateCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if changes.CreatedExternal {
		t.Fatal("should keep existing external key")
	}
	if !changes.CreatedInternal {
		t.Fatal("expected internal key migration")
	}
	if creds.ExternalAPIKey != legacy.APIKey {
		t.Fatalf("external key changed: got %q", creds.ExternalAPIKey)
	}
	if len(creds.InternalAPIKey) != 32 {
		t.Fatalf("expected generated internal key, got len %d", len(creds.InternalAPIKey))
	}

	creds2, changes2, err := auth.LoadOrCreateCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if changes2.CreatedInternal {
		t.Fatal("internal key should persist after migration")
	}
	if creds2.InternalAPIKey != creds.InternalAPIKey {
		t.Fatal("internal key should be stable")
	}
}

func TestLoadOrCreateCredentials_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")
	if err := os.WriteFile(path, []byte(`{bad`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := auth.LoadOrCreateCredentials(path)
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

	t.Run("valid apikey query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?apikey="+apiKey, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
	})
}

func TestQueryAPIKeyAuth(t *testing.T) {
	const apiKey = "test-api-key-secret"
	log := testLogger()
	handler := auth.QueryAPIKeyAuth(apiKey, log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("missing apikey", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/preview/1", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("wrong apikey", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/preview/1?apikey=wrong", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("valid apikey", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/preview/1?apikey="+apiKey, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("valid bearer", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/preview/1", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
	})
}

func TestKeyRoleSeparation(t *testing.T) {
	const externalKey = "external-key-for-rest-api"
	const internalKey = "internal-key-for-ingest"
	log := testLogger()

	external := auth.BearerAuth(externalKey, log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	internal := auth.InternalBearerAuth(internalKey, log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("external rejects internal key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/ingest", nil)
		req.Header.Set("Authorization", "Bearer "+internalKey)
		rr := httptest.NewRecorder()
		external.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("internal rejects external key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/ingest", nil)
		req.Header.Set("Authorization", "Bearer "+externalKey)
		rr := httptest.NewRecorder()
		internal.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("external accepts external key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/emails", nil)
		req.Header.Set("Authorization", "Bearer "+externalKey)
		rr := httptest.NewRecorder()
		external.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
	})

	t.Run("internal accepts internal key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/ingest", nil)
		req.Header.Set("Authorization", "Bearer "+internalKey)
		rr := httptest.NewRecorder()
		internal.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
	})
}
