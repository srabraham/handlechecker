package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a simple per-client-IP token-bucket limiter. Each IP gets a
// bucket that refills at `rate` tokens per second up to `burst` tokens; each
// request spends one token. It is safe for concurrent use, and evicts buckets
// that have been idle for a while so the map can't grow without bound. No
// external dependency — deliberately minimal, to take the edge off abuse of the
// public /api/check endpoint, not to be a precise quota system.
type rateLimiter struct {
	rate  float64
	burst float64

	mu       sync.Mutex
	visitors map[string]*visitor
}

type visitor struct {
	tokens float64
	last   time.Time
}

const visitorTTL = 10 * time.Minute

func newRateLimiter(ratePerSec, burst float64) *rateLimiter {
	rl := &rateLimiter{
		rate:     ratePerSec,
		burst:    burst,
		visitors: make(map[string]*visitor),
	}
	go rl.cleanupLoop()
	return rl
}

// allow reports whether a request from ip may proceed, spending one token.
func (rl *rateLimiter) allow(ip string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, ok := rl.visitors[ip]
	if !ok {
		// A new visitor arrives with a full bucket, then spends one token.
		rl.visitors[ip] = &visitor{tokens: rl.burst - 1, last: now}
		return true
	}
	// Refill for the time elapsed since this visitor's last request, capped at
	// burst, then try to spend a token.
	v.tokens += now.Sub(v.last).Seconds() * rl.rate
	if v.tokens > rl.burst {
		v.tokens = rl.burst
	}
	v.last = now
	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

// cleanupLoop periodically drops idle visitors so the map stays bounded.
func (rl *rateLimiter) cleanupLoop() {
	for range time.Tick(visitorTTL) {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.last) > visitorTTL {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// middleware wraps h, rejecting requests from a client IP that has exceeded its
// rate with 429 Too Many Requests.
func (rl *rateLimiter) middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded; slow down", http.StatusTooManyRequests)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// clientIP returns the client's IP from the connection's remote address. The
// server terminates TLS itself, so RemoteAddr is the real client; if you instead
// run it behind a trusted reverse proxy, that proxy should be the one enforcing
// limits (X-Forwarded-For is caller-spoofable and deliberately not trusted here).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
