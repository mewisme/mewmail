package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"mewmail/api/internal/httputil"
)

// Recoverer catches panics and returns 500.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						"error", rec,
						"request_id", GetRequestID(r.Context()),
						"stack", string(debug.Stack()),
					)
					httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
