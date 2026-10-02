package main

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLog runs fn with the standard logger redirected to a buffer and returns
// what was written, so tests can assert on access-log output.
func captureLog(fn func()) string {
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	}()
	fn()
	return buf.String()
}

func TestAuthMiddlewareLogsExplicitLoginAttempts(t *testing.T) {
	h := authMiddleware([]string{"alpha"}, okHandler)

	// logForRequest runs one request through the middleware and returns whatever
	// it wrote to the log.
	logForRequest := func(setup func(*http.Request)) string {
		return captureLog(func() {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			setup(r)
			h.ServeHTTP(httptest.NewRecorder(), r)
		})
	}

	// A correct key supplied via header is a granted login attempt.
	out := logForRequest(func(r *http.Request) {
		r.Header.Set("X-Access-Key", "alpha")
	})
	if !strings.Contains(out, "login attempt granted") {
		t.Errorf("granted via header: want a granted login attempt, got %q", out)
	}

	// A wrong key supplied via header is a denied login attempt.
	out = logForRequest(func(r *http.Request) {
		r.Header.Set("X-Access-Key", "nope")
	})
	if !strings.Contains(out, "login attempt denied") {
		t.Errorf("denied via header: want a denied login attempt, got %q", out)
	}

	// A session cookie is "already logged in", not a fresh login attempt.
	out = logForRequest(func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: accessCookieName, Value: "alpha"})
	})
	if strings.Contains(out, "login attempt") {
		t.Errorf("cookie should not log a login attempt, got %q", out)
	}

	// A bare page view presents no credential, so it logs nothing.
	out = logForRequest(func(*http.Request) {})
	if strings.Contains(out, "login attempt") {
		t.Errorf("bare page view should not log a login attempt, got %q", out)
	}
}

func TestLoginLogOmitsKeyAndQuery(t *testing.T) {
	// The key arrives in the query string; the log must record the path only,
	// never the secret.
	const secret = "s3cr3t-key"
	h := authMiddleware([]string{secret}, okHandler)
	out := captureLog(func() {
		r := httptest.NewRequest(http.MethodGet, "/?key="+secret, nil)
		h.ServeHTTP(httptest.NewRecorder(), r)
	})
	if !strings.Contains(out, "login attempt granted") {
		t.Fatalf("expected a granted login attempt, got %q", out)
	}
	if strings.Contains(out, secret) {
		t.Fatalf("log leaked the access key: %q", out)
	}
}

// okHandler is a stand-in for the real mux: it 200s anything that reaches it, so
// tests can tell "passed the gate" from "blocked by the gate".
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestAuthMiddlewareDisabledWhenNoKeys(t *testing.T) {
	h := authMiddleware(nil, okHandler)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("no keys configured should leave the site open, got %d", rec.Code)
	}
}

func TestAuthMiddlewareBlocksAndAccepts(t *testing.T) {
	keys := []string{"alpha", "bravo"}
	h := authMiddleware(keys, okHandler)

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
	h := authMiddleware([]string{"alpha"}, okHandler)
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
			if c.Value != "alpha" || !c.HttpOnly || !c.Secure {
				t.Fatalf("cookie should hold the key and be HttpOnly and Secure, got %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("a valid ?key= should set the access cookie")
	}
}

func TestAuthMiddlewareServesAccessPageToBrowsers(t *testing.T) {
	h := authMiddleware([]string{"alpha"}, okHandler)

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
	h := authMiddleware([]string{"alpha"}, okHandler)
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
	h := authMiddleware([]string{"alpha"}, okHandler)
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
	h := authMiddleware([]string{"alpha"}, okHandler)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/check?key=alpha", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("valid ?key= POST should pass through, got %d", rec.Code)
	}
}
