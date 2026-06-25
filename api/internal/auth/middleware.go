package auth

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"mewmail/api/internal/httputil"
)

type contextKey string

const apiKeyContextKey contextKey = "apiKey"

func validAPIKey(provided, expected string) bool {
	return provided != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// BearerAuth validates Authorization: Bearer <apiKey>.
func BearerAuth(apiKey string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearer(r.Header.Get("Authorization"))
			if !ok || !validAPIKey(token, apiKey) {
				log.Warn("auth failure", "path", r.URL.Path, "remote", r.RemoteAddr)
				httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// QueryAPIKeyAuth validates ?apikey=<apiKey> (browser-friendly links).
func QueryAPIKeyAuth(apiKey string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !validAPIKey(r.URL.Query().Get("apikey"), apiKey) {
				log.Warn("auth failure", "path", r.URL.Path, "remote", r.RemoteAddr)
				httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// InternalBearerAuth validates the internal ingestion token.
func InternalBearerAuth(token string, log *slog.Logger) func(http.Handler) http.Handler {
	return BearerAuth(token, log)
}

func extractBearer(h string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	t := strings.TrimSpace(h[len(prefix):])
	return t, t != ""
}

// WithAPIKey stores the key in context (for tests).
func WithAPIKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, apiKeyContextKey, key)
}
