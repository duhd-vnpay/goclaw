package http

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter provides per-IP rate limiting for HTTP endpoints.
// Uses a sliding window counter stored in-memory.
type RateLimiter struct {
	mu       sync.Mutex
	counters map[string]*windowCounter
	limit    int           // max requests per window
	window   time.Duration // sliding window duration
}

type windowCounter struct {
	count    int
	windowAt time.Time
}

// NewRateLimiter creates a rate limiter with the given limit per window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		counters: make(map[string]*windowCounter),
		limit:    limit,
		window:   window,
	}
	// Periodic cleanup of stale entries (every 5 minutes).
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			rl.cleanup()
		}
	}()
	return rl
}

// Allow checks if a request from the given key is allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	c, ok := rl.counters[key]
	if !ok || now.Sub(c.windowAt) >= rl.window {
		rl.counters[key] = &windowCounter{count: 1, windowAt: now}
		return true
	}
	c.count++
	return c.count <= rl.limit
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-rl.window * 2)
	for k, c := range rl.counters {
		if c.windowAt.Before(cutoff) {
			delete(rl.counters, k)
		}
	}
}

// RateLimitMiddleware wraps an http.Handler with per-IP rate limiting.
func RateLimitMiddleware(next http.Handler, limiter *RateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractClientIP(r)
		if !limiter.Allow(ip) {
			slog.Warn("security.rate_limit_exceeded",
				"ip", ip,
				"path", r.URL.Path,
				"method", r.Method,
			)
			w.Header().Set("Retry-After", "60")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractClientIP returns the client IP from X-Forwarded-For, X-Real-IP, or RemoteAddr.
func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (client IP).
		if i := len(xff); i > 0 {
			parts := splitFirst(xff, ',')
			return trimSpace(parts)
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return trimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func splitFirst(s string, sep byte) string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return s[:i]
		}
	}
	return s
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}
