package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRateLimiterBurst checks that a bucket allows up to `burst` requests then
// denies, using rate 0 so no tokens refill during the test.
func TestRateLimiterBurst(t *testing.T) {
	rl := newRateLimiter(0, 3)
	got := []bool{rl.allow("1.2.3.4"), rl.allow("1.2.3.4"), rl.allow("1.2.3.4"), rl.allow("1.2.3.4")}
	want := []bool{true, true, true, false}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("allow #%d = %v, want %v (got sequence %v)", i+1, got[i], want[i], got)
		}
	}
}

// TestRateLimiterPerIP checks that each IP gets its own independent bucket.
func TestRateLimiterPerIP(t *testing.T) {
	rl := newRateLimiter(0, 1)
	if !rl.allow("1.1.1.1") {
		t.Fatal("first request from 1.1.1.1 should be allowed")
	}
	if rl.allow("1.1.1.1") {
		t.Error("second request from 1.1.1.1 should be denied (burst 1)")
	}
	if !rl.allow("2.2.2.2") {
		t.Error("a different IP should have its own bucket")
	}
}

// TestRateLimitMiddleware checks the 429 response once the bucket is empty.
func TestRateLimitMiddleware(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := newRateLimiter(0, 1).middleware(ok)

	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/check", nil)
	req.RemoteAddr = "9.9.9.9:5555"
	h.ServeHTTP(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Errorf("second request: got %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("expected a Retry-After header on the 429 response")
	}
}

func TestClientIP(t *testing.T) {
	cases := map[string]string{
		"203.0.113.7:54321": "203.0.113.7",
		"[2001:db8::1]:443": "2001:db8::1",
		"no-port":           "no-port",
	}
	for remote, want := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if got := clientIP(r); got != want {
			t.Errorf("clientIP(%q) = %q, want %q", remote, got, want)
		}
	}
}
