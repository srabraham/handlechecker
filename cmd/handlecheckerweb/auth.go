package main

import (
	"crypto/subtle"
	_ "embed" // for the //go:embed access.html directive
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// accessCookieName is the cookie that caches a valid access key so the page's
// subresource loads and API fetches (which don't carry ?p=) are authorized, and
// so a bare-URL visit can be redirected back onto its ?p= form. HttpOnly, so
// client JS can't read it.
const accessCookieName = "hc_access"

// accessKeyParam is the query-string parameter a visitor can use to present a
// key in a shareable link, e.g. https://host/?p=SECRET. It is deliberately kept
// in the address bar so the URL can be bookmarked or shared to let someone in.
const accessKeyParam = "p"

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
// ?p= query parameter, the X-Access-Key header, the HTTP Basic Auth password
// (username ignored), or the hc_access cookie. On a valid ?p=, the key is also
// stored in an HttpOnly cookie for the page's subresources and API calls. A
// browser navigation authorized only by the cookie is redirected to the same
// URL with ?p= added, so the address bar always shows a shareable link.
//
// When keys is empty, authentication is disabled and the handler is returned
// unwrapped (preserving the open local/dev behavior).
func authMiddleware(keys []string, next http.Handler) http.Handler {
	if len(keys) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented, src := presentedKey(r)
		ok := presented != "" && keyMatches(keys, presented)

		// Log explicit login attempts (a key supplied via query/header/basic, not
		// a session cookie). Only the client IP, the target path, and the outcome
		// are recorded — never the key itself or any request body.
		if src.isLoginAttempt() {
			outcome := "denied"
			if ok {
				outcome = "granted"
			}
			log.Printf("login attempt %s ip=%s path=%s", outcome, clientIP(r), r.URL.Path)
		}

		if ok {
			if src == keyQuery {
				http.SetCookie(w, &http.Cookie{
					Name:     accessCookieName,
					Value:    presented,
					Path:     "/",
					HttpOnly: true,
					// Always Secure: TLS is terminated upstream, so r.TLS is nil
					// even though the browser is on HTTPS.
					Secure:   true,
					SameSite: http.SameSiteLaxMode,
				})
			} else if src == keyCookie && wantsHTML(r) {
				q := r.URL.Query()
				q.Set(accessKeyParam, presented)
				withKey := *r.URL
				withKey.RawQuery = q.Encode()
				http.Redirect(w, r, withKey.RequestURI(), http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
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
			writeAccessPage(w, r, msg)
			return
		}
		// API / scripted clients: a plain 401, advertising that Basic Auth (key
		// as password) is an accepted credential.
		w.Header().Set("WWW-Authenticate", `Basic realm="handlechecker"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// keySource identifies where a request's access key came from.
type keySource int

const (
	keyNone   keySource = iota // no key presented
	keyQuery                   // ?p= query parameter
	keyHeader                  // X-Access-Key header
	keyBasic                   // HTTP Basic Auth password
	keyCookie                  // hc_access session cookie (already logged in)
)

// isLoginAttempt reports whether the source is an explicit credential — i.e. a
// fresh login attempt — as opposed to a session cookie (already authenticated)
// or no key at all (just loading a page).
func (s keySource) isLoginAttempt() bool {
	return s == keyQuery || s == keyHeader || s == keyBasic
}

// presentedKey extracts the access key a request offers, trying the query
// parameter, the X-Access-Key header, Basic Auth password, then the cookie, and
// reports which source supplied it (keyNone if none).
func presentedKey(r *http.Request) (key string, src keySource) {
	if k := r.URL.Query().Get(accessKeyParam); k != "" {
		return k, keyQuery
	}
	if k := r.Header.Get("X-Access-Key"); k != "" {
		return k, keyHeader
	}
	if _, pass, ok := r.BasicAuth(); ok && pass != "" {
		return pass, keyBasic
	}
	if c, err := r.Cookie(accessCookieName); err == nil && c.Value != "" {
		return unescapeCookie(c.Value), keyCookie
	}
	return "", keyNone
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
// static/style.css. The form does a plain GET, so submitting sets ?p=… and
// re-enters authMiddleware, which on a valid key stores the cookie and serves
// the app. Action is the current path so the visitor lands where they were
// headed.
//
//go:embed access.html
var accessPageHTML string

var accessPageTemplate = template.Must(template.New("access").Parse(accessPageHTML))

// writeAccessPage renders the access-key page with a 401. errMsg, when
// non-empty, is shown above the form. No WWW-Authenticate header is set here, so
// browsers show this page rather than their native credential dialog.
func writeAccessPage(w http.ResponseWriter, r *http.Request, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	// Action is r.URL.Path (always starts with "/"); html/template applies its
	// attribute/URL escaping, so a plain string is both safe and correct here.
	_ = accessPageTemplate.Execute(w, struct {
		Action string
		Error  string
	}{
		Action: r.URL.Path,
		Error:  errMsg,
	})
}
