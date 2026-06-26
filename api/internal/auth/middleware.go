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

// ValidAPIKey reports whether provided matches expected (constant-time).
func ValidAPIKey(provided, expected string) bool {
	return validAPIKey(provided, expected)
}

// BearerAuth validates Authorization: Bearer <apiKey> or ?apikey=<apiKey>.
func BearerAuth(apiKey string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !ValidRequestAPIKey(r, apiKey) {
				log.Warn("auth failure", "path", r.URL.Path, "remote", r.RemoteAddr)
				httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// QueryAPIKeyAuth validates ?apikey=<apiKey> or Authorization: Bearer <apiKey>.
func QueryAPIKeyAuth(apiKey string, log *slog.Logger) func(http.Handler) http.Handler {
	return BearerAuth(apiKey, log)
}

// ValidRequestAPIKey reports whether the request carries the external API key via Bearer header or ?apikey=.
func ValidRequestAPIKey(r *http.Request, expected string) bool {
	if token, ok := extractBearer(r.Header.Get("Authorization")); ok && validAPIKey(token, expected) {
		return true
	}
	return validAPIKey(r.URL.Query().Get("apikey"), expected)
}

// InternalBearerAuth validates the internal ingestion token (Bearer header or ?apikey=).
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
