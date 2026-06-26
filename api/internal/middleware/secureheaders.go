package middleware

import (
	"net/http"
	"strings"
)

// SecureHeaders sets common security response headers.
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/ui/") || strings.HasPrefix(r.URL.Path, "/preview") || strings.Contains(r.URL.Path, "/swagger") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		next.ServeHTTP(w, r)
	})
}
