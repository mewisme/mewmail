package middleware

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestID wraps chi's RequestID middleware.
func RequestID(next http.Handler) http.Handler {
	return middleware.RequestID(next)
}

// GetRequestID returns the request ID from context.
func GetRequestID(ctx context.Context) string {
	return middleware.GetReqID(ctx)
}
