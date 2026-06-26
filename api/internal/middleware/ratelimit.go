package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ponytail: global mutex + per-IP counter; ceiling ~1000 IPs, upgrade to x/time/rate per IP if needed.
type rateLimiter struct {
	mu      sync.Mutex
	visits  map[string]*bucket
	limit   int
	window  time.Duration
	maxKeys int
}

type bucket struct {
	count   int
	resetAt time.Time
}

// RateLimit allows limit requests per IP per window.
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		visits:  make(map[string]*bucket),
		limit:   limit,
		window:  window,
		maxKeys: 1000,
	}
	return rl.middleware
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i >= 0 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.visits[key]
	if !ok || now.After(b.resetAt) {
		if !ok {
			if len(rl.visits) >= rl.maxKeys {
				rl.evictExpired(now)
			}
			if len(rl.visits) >= rl.maxKeys {
				return false
			}
		}
		rl.visits[key] = &bucket{count: 1, resetAt: now.Add(rl.window)}
		return true
	}
	if b.count >= rl.limit {
		return false
	}
	b.count++
	return true
}

func (rl *rateLimiter) evictExpired(now time.Time) {
	for k, b := range rl.visits {
		if now.After(b.resetAt) {
			delete(rl.visits, k)
		}
	}
}
