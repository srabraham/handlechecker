package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// okHandler is a stand-in for the real mux: it 200s anything that reaches it, so
// tests can tell "passed the gate" from "blocked by the gate".
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestAuthMiddlewareDisabledWhenNoKeys(t *testing.T) {
	h := authMiddleware(nil, nil, okHandler)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("no keys configured should leave the site open, got %d", rec.Code)
	}
}

func TestAuthMiddlewareBlocksAndAccepts(t *testing.T) {
	keys := []string{"alpha", "bravo"}
	h := authMiddleware(keys, nil, okHandler)

	tests := []struct {
		name     string
		setup    func(*http.Request)
		wantCode int
	}{
		{"no credentials", func(*http.Request) {}, http.StatusUnauthorized},
		{"wrong key", func(r *http.Request) {
			r.URL.RawQuery = "key=nope"
		}, http.StatusUnauthorized},
		{"valid header", func(r *http.Request) {
			r.Header.Set("X-Access-Key", "bravo")
		}, http.StatusOK},
		{"valid basic auth password", func(r *http.Request) {
			r.SetBasicAuth("anyone", "alpha")
		}, http.StatusOK},
		{"valid cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: accessCookieName, Value: "alpha"})
		}, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/check", nil)
			tc.setup(r)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != tc.wantCode {
				t.Fatalf("got %d, want %d", rec.Code, tc.wantCode)
			}
		})
	}
}

func TestAuthMiddlewareQueryKeySetsCookieAndRedirects(t *testing.T) {
	h := authMiddleware([]string{"alpha"}, nil, okHandler)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?key=alpha&foo=bar", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("a valid ?key= GET should redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/?foo=bar" {
		t.Fatalf("redirect should strip key and keep other params, got %q", loc)
	}
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == accessCookieName {
			found = true
			if c.Value != "alpha" || !c.HttpOnly {
				t.Fatalf("cookie should hold the key and be HttpOnly, got %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("a valid ?key= should set the access cookie")
	}
}

func TestAuthMiddlewareServesAccessPageToBrowsers(t *testing.T) {
	h := authMiddleware([]string{"alpha"}, nil, okHandler)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept", "text/html,application/xhtml+xml")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected an HTML page, got Content-Type %q", ct)
	}
	if rec.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("the HTML page must not set WWW-Authenticate, else browsers show the native dialog")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Access required") || !strings.Contains(body, `name="key"`) {
		t.Fatalf("page missing heading or key field:\n%s", body)
	}
	if strings.Contains(body, "wasn’t recognized") {
		t.Fatal("no error message expected on a first visit with no key")
	}
}

func TestAuthMiddlewareAccessPageShowsErrorOnWrongKey(t *testing.T) {
	h := authMiddleware([]string{"alpha"}, nil, okHandler)
	r := httptest.NewRequest(http.MethodGet, "/?key=wrong", nil)
	r.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if !strings.Contains(rec.Body.String(), "wasn’t recognized") {
		t.Fatalf("a rejected key should surface an error message:\n%s", rec.Body.String())
	}
}

func TestAuthMiddlewareApiGetsPlain401NotPage(t *testing.T) {
	// Even with an HTML Accept header, /api/ paths are programmatic: a bare 401,
	// never the page (which would corrupt a fetch caller's error handling).
	h := authMiddleware([]string{"alpha"}, nil, okHandler)
	r := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	r.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Access required") {
		t.Fatal("/api/ paths must not receive the HTML access page")
	}
}

func TestAuthMiddlewareQueryKeyOnPostDoesNotRedirect(t *testing.T) {
	// API calls are POSTs; a redirect would drop the body. The key still works,
	// it just shouldn't turn into a 303.
	h := authMiddleware([]string{"alpha"}, nil, okHandler)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/check?key=alpha", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("valid ?key= POST should pass through, got %d", rec.Code)
	}
}

// wrongGuess issues one wrong-key request from a fixed IP and returns its code.
func wrongGuess(h http.Handler, ip string) int {
	r := httptest.NewRequest(http.MethodGet, "/api/check?key=nope", nil)
	r.RemoteAddr = ip + ":12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code
}

func TestAuthMiddlewareThrottlesWrongGuesses(t *testing.T) {
	// No refill during the test (rate 0): exactly `burst` guesses get through as
	// 401s, then the bucket is empty and further guesses get 429.
	const burst = 3
	h := authMiddleware([]string{"alpha"}, newRateLimiter(0, burst), okHandler)

	for i := 0; i < burst; i++ {
		if got := wrongGuess(h, "10.0.0.1"); got != http.StatusUnauthorized {
			t.Fatalf("guess %d: got %d, want 401 (still within budget)", i+1, got)
		}
	}
	if got := wrongGuess(h, "10.0.0.1"); got != http.StatusTooManyRequests {
		t.Fatalf("guess past budget: got %d, want 429", got)
	}

	// The throttle is per-IP: a different client is unaffected.
	if got := wrongGuess(h, "10.0.0.2"); got != http.StatusUnauthorized {
		t.Fatalf("other IP: got %d, want 401", got)
	}
}

func TestAuthMiddlewareCorrectKeyBypassesThrottle(t *testing.T) {
	// An exhausted bucket must never lock out the real key — only wrong guesses
	// are throttled.
	h := authMiddleware([]string{"alpha"}, newRateLimiter(0, 1), okHandler)

	if got := wrongGuess(h, "10.0.0.3"); got != http.StatusUnauthorized {
		t.Fatalf("first wrong guess: got %d, want 401", got)
	}
	if got := wrongGuess(h, "10.0.0.3"); got != http.StatusTooManyRequests {
		t.Fatalf("budget should be spent: got %d, want 429", got)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	r.Header.Set("X-Access-Key", "alpha")
	r.RemoteAddr = "10.0.0.3:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct key while throttled should still pass, got %d", rec.Code)
	}
}

func TestAuthMiddlewarePageViewsDoNotSpendBudget(t *testing.T) {
	// Loading the access page (no key presented) is not a guess; it must not burn
	// the failure budget, or a few refreshes would lock a user out before they
	// even type anything.
	h := authMiddleware([]string{"alpha"}, newRateLimiter(0, 1), okHandler)

	for i := 0; i < 5; i++ {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept", "text/html")
		r.RemoteAddr = "10.0.0.4:12345"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("page view %d: got %d, want 401 (not throttled)", i+1, rec.Code)
		}
	}
	// Budget intact: the first real wrong guess is still a 401, not a 429.
	if got := wrongGuess(h, "10.0.0.4"); got != http.StatusUnauthorized {
		t.Fatalf("first guess after page views: got %d, want 401", got)
	}
}

func TestAuthMiddlewareThrottledPageIsFriendlyAndSetsRetryAfter(t *testing.T) {
	h := authMiddleware([]string{"alpha"}, newRateLimiter(0, 1), okHandler)
	// Spend the single-guess budget, then the next wrong guess is throttled.
	if got := wrongGuess(h, "10.0.0.5"); got != http.StatusUnauthorized {
		t.Fatalf("first guess: got %d, want 401", got)
	}
	r := httptest.NewRequest(http.MethodGet, "/?key=nope", nil)
	r.Header.Set("Accept", "text/html")
	r.RemoteAddr = "10.0.0.5:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("throttled response should advertise Retry-After")
	}
	if body := rec.Body.String(); !strings.Contains(body, "Too many attempts") {
		t.Fatalf("expected the friendly throttle page, got:\n%s", body)
	}
}
