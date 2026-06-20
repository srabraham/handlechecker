package main

import (
	"crypto/subtle"
	_ "embed" // for the //go:embed access.html directive
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// accessCookieName is the cookie that caches a valid access key so it drops out
// of the URL after the first ?key= visit. HttpOnly, so client JS can't read it.
const accessCookieName = "hc_access"

// accessKeyParam is the query-string parameter a visitor can use to present a
// key in a shareable link, e.g. https://host/?key=SECRET.
const accessKeyParam = "key"

// Failed-guess throttle: each wrong key from a client IP spends one token from a
// bucket of authFailBurst, refilling at authFailRate per second. So an IP gets a
// short burst of attempts, then is limited to roughly one guess per
// 1/authFailRate seconds — enough to blunt brute-forcing of the shared key
// without locking out a fumbling legitimate user for long. Correct keys never
// touch the bucket. authRetryAfterSeconds is the Retry-After hint sent when
// throttled (≈ the refill interval).
const (
	authFailRate          = 0.2 // 1 token per 5s
	authFailBurst         = 5
	authRetryAfterSeconds = "5"
)

// loadAccessKeys parses the ACCESS_KEYS environment variable into the set of
// valid keys: a comma-separated list, with surrounding whitespace trimmed and
// empty entries dropped. An empty result means authentication is disabled.
func loadAccessKeys() []string {
	var keys []string
	for _, k := range strings.Split(os.Getenv("ACCESS_KEYS"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// authMiddleware gates every request behind a shared access key. A request is
// allowed if it presents a key matching any in keys via any of, in order: the
// ?key= query parameter, the X-Access-Key header, the HTTP Basic Auth password
// (username ignored), or the hc_access cookie. On a valid ?key=, the key is
// stored in an HttpOnly cookie and — for top-level GET navigations — the visitor
// is redirected to the same URL with the key stripped, so it doesn't linger in
// the address bar, history, or Referer headers.
//
// When keys is empty, authentication is disabled and the handler is returned
// unwrapped (preserving the open local/dev behavior).
//
// fails, if non-nil, throttles wrong guesses per client IP: each rejected key
// spends a token, and once an IP's budget is spent further guesses get 429 until
// it refills. A correct key never touches the limiter (a legit user who finally
// types it right is never locked out), and a bare visit with no key presented
// costs nothing (loading the page is not a guess).
func authMiddleware(keys []string, fails *rateLimiter, next http.Handler) http.Handler {
	if len(keys) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented, fromQuery := presentedKey(r)

		if presented != "" && keyMatches(keys, presented) {
			if fromQuery {
				// Persist the key so subsequent requests need not carry it, and
				// scrub it from the URL on plain navigations.
				http.SetCookie(w, &http.Cookie{
					Name:     accessCookieName,
					Value:    presented,
					Path:     "/",
					HttpOnly: true,
					Secure:   r.TLS != nil,
					SameSite: http.SameSiteLaxMode,
				})
				if r.Method == http.MethodGet {
					q := r.URL.Query()
					q.Del(accessKeyParam)
					clean := *r.URL
					clean.RawQuery = q.Encode()
					http.Redirect(w, r, clean.RequestURI(), http.StatusSeeOther)
					return
				}
			}
			next.ServeHTTP(w, r)
			return
		}

		// Unauthorized. Count an actual wrong guess (a presented-but-invalid key)
		// against this IP's failure budget; once spent, reject further guesses
		// cheaply with 429. A request with no key presented is just a page view,
		// so it neither spends budget nor counts as throttled.
		if presented != "" && fails != nil && !fails.allow(clientIP(r)) {
			writeThrottled(w, r)
			return
		}

		if wantsHTML(r) {
			// Browser navigation: show the friendly access-key page instead of
			// the gray native Basic Auth dialog. A non-empty presented key means
			// the visitor just submitted a wrong one — flag it.
			msg := ""
			if presented != "" {
				msg = "That access key wasn’t recognized. Try again."
			}
			writeAccessPage(w, r, http.StatusUnauthorized, msg, false)
			return
		}
		// API / scripted clients: a plain 401, advertising that Basic Auth (key
		// as password) is an accepted credential.
		w.Header().Set("WWW-Authenticate", `Basic realm="handlechecker"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// presentedKey extracts the access key a request offers, trying the query
// parameter, the X-Access-Key header, Basic Auth password, then the cookie. The
// boolean reports whether it came from the query parameter (the only source that
// warrants caching it in a cookie and scrubbing the URL).
func presentedKey(r *http.Request) (key string, fromQuery bool) {
	if k := r.URL.Query().Get(accessKeyParam); k != "" {
		return k, true
	}
	if k := r.Header.Get("X-Access-Key"); k != "" {
		return k, false
	}
	if _, pass, ok := r.BasicAuth(); ok && pass != "" {
		return pass, false
	}
	if c, err := r.Cookie(accessCookieName); err == nil && c.Value != "" {
		return unescapeCookie(c.Value), false
	}
	return "", false
}

// unescapeCookie reverses any percent-encoding net/http applied when writing the
// cookie value, so the round-tripped key compares equal to the configured one.
func unescapeCookie(v string) string {
	if unq, err := url.QueryUnescape(v); err == nil {
		return unq
	}
	return v
}

// keyMatches reports whether presented equals any configured key, comparing in
// constant time to avoid leaking key contents through timing. Every candidate is
// compared (no early return) so the work doesn't depend on which key matched.
func keyMatches(keys []string, presented string) bool {
	var matched int
	p := []byte(presented)
	for _, k := range keys {
		matched |= subtle.ConstantTimeCompare([]byte(k), p)
	}
	return matched == 1
}

// wantsHTML reports whether an unauthorized request should get the human-facing
// access page rather than a bare 401. True for top-level browser navigations
// (GETs whose Accept advertises HTML); false for the API and for fetch/XHR/curl
// callers, which want a machine-readable status. Anything under /api/ is always
// treated as a programmatic caller.
func wantsHTML(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// accessPageHTML is the standalone "enter access key" page, embedded from
// access.html. It must be fully self-contained — the real stylesheet lives
// behind this very gate — so the palette is inlined there to match
// static/style.css. The form does a plain GET, so submitting sets ?key=… and
// re-enters authMiddleware, which on a valid key stores the cookie and redirects
// on through. Action is the current path so the visitor lands where they were
// headed.
//
//go:embed access.html
var accessPageHTML string

var accessPageTemplate = template.Must(template.New("access").Parse(accessPageHTML))

// writeAccessPage renders the access-key page with the given status (401 for a
// normal prompt/rejection, 429 when throttled — the body renders either way).
// errMsg, when non-empty, is shown above the form; disabled greys out the form
// (used while throttled, so the visitor waits rather than burning guesses). No
// WWW-Authenticate header is set here, so browsers show this page rather than
// their native credential dialog.
func writeAccessPage(w http.ResponseWriter, r *http.Request, status int, errMsg string, disabled bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	// Action is r.URL.Path (always starts with "/"); html/template applies its
	// attribute/URL escaping, so a plain string is both safe and correct here.
	_ = accessPageTemplate.Execute(w, struct {
		Action   string
		Error    string
		Disabled bool
	}{
		Action:   r.URL.Path,
		Error:    errMsg,
		Disabled: disabled,
	})
}

// writeThrottled responds to a wrong guess from an IP that has exhausted its
// failure budget: a friendly 429 page for browsers, a bare 429 for programmatic
// callers. Either way it advertises Retry-After so clients know to back off.
func writeThrottled(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", authRetryAfterSeconds)
	if wantsHTML(r) {
		writeAccessPage(w, r, http.StatusTooManyRequests,
			"Too many attempts. Please wait a few seconds and try again.", true)
		return
	}
	http.Error(w, "too many failed attempts; slow down", http.StatusTooManyRequests)
}
