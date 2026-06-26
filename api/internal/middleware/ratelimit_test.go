package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.5:54321"
	if got := clientIP(r); got != "203.0.113.5" {
		t.Fatalf("RemoteAddr: got %q", got)
	}

	r.Header.Set("X-Forwarded-For", "198.51.100.2, 10.0.0.1")
	if got := clientIP(r); got != "198.51.100.2" {
		t.Fatalf("X-Forwarded-For: got %q", got)
	}
}

func TestRateLimitPerIPNotPort(t *testing.T) {
	rl := &rateLimiter{
		visits:  make(map[string]*bucket),
		limit:   2,
		window:  time.Minute,
		maxKeys: 1000,
	}
	if !rl.allow("203.0.113.5") || !rl.allow("203.0.113.5") {
		t.Fatal("expected two requests from same IP")
	}
	if rl.allow("203.0.113.5") {
		t.Fatal("third request from same IP should be limited")
	}
	if !rl.allow("203.0.113.6") {
		t.Fatal("different IP should not be limited yet")
	}
}

func TestRateLimitEvictsExpiredKeys(t *testing.T) {
	rl := &rateLimiter{
		visits:  make(map[string]*bucket),
		limit:   10,
		window:  time.Minute,
		maxKeys: 2,
	}
	past := time.Now().Add(-time.Minute)
	rl.visits["old"] = &bucket{count: 99, resetAt: past}
	rl.visits["keep"] = &bucket{count: 1, resetAt: time.Now().Add(time.Minute)}

	if !rl.allow("new") {
		t.Fatal("expected expired key eviction to make room for new IP")
	}
	if _, ok := rl.visits["old"]; ok {
		t.Fatal("expired key should be removed")
	}
}

func TestRateLimitMiddlewareUsesClientIP(t *testing.T) {
	h := RateLimit(1, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := func(addr string) *http.Response {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = addr
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr.Result()
	}
	if req("203.0.113.5:11111").StatusCode != http.StatusOK {
		t.Fatal("first request should pass")
	}
	if req("203.0.113.5:22222").StatusCode != http.StatusTooManyRequests {
		t.Fatal("second request from same IP different port should be limited")
	}
}
