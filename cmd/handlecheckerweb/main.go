// Command handlecheckerweb serves a small web interface for vetting proposed
// Burning Man radio callsigns one at a time against a baseline of reserved
// terms and existing handles, reusing the same confusability engine as the CLI.
//
// The server is stateless: the browser holds the lists and re-sends them with
// each check. The only persistent server-side state is the in-process phoneme
// cache in internal/phonetic, which stays warm for the life of the process.
//
// Usage:
//
//	# Local / behind a reverse proxy: plain HTTP.
//	handlecheckerweb --addr :8080
//
//	# Public on the internet: terminate TLS with auto-provisioned Let's Encrypt
//	# certificates (no certbot needed — the binary speaks ACME itself). Bind 80
//	# and 443, and point your domain's A/AAAA records at this host.
//	handlecheckerweb --tls-domain handles.example.org --tls-email you@example.org
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"github.com/srabraham/handlechecker/internal/checker"
)

//go:embed static
var staticFS embed.FS

// letsEncryptStagingURL is the ACME directory for Let's Encrypt's staging
// environment: untrusted certificates but far higher rate limits, for testing
// the provisioning flow without burning the production quota.
const letsEncryptStagingURL = "https://acme-staging-v02.api.letsencrypt.org/directory"

func main() {
	addr := flag.String("addr", ":8080", "address to serve plain HTTP on (used when -tls-domain is empty)")
	tlsDomain := flag.String("tls-domain", "", "comma-separated domain(s) to obtain Let's Encrypt certificates for; setting this enables HTTPS")
	tlsEmail := flag.String("tls-email", "", "contact email to register with Let's Encrypt (recommended: receive expiry warnings)")
	tlsCache := flag.String("tls-cache", "certs", "directory to cache obtained certificates and the ACME account key")
	httpsAddr := flag.String("https-addr", ":443", "address to serve HTTPS on when -tls-domain is set")
	httpAddr := flag.String("http-addr", ":80", "address for ACME HTTP-01 challenges and HTTP→HTTPS redirects when -tls-domain is set")
	tlsStaging := flag.Bool("tls-staging", false, "use the Let's Encrypt staging environment (untrusted certs, high rate limits) for testing")
	rateLimit := flag.Float64("rate-limit", 2, "max sustained POST /api/check requests per second per client IP (burst is 5x, min 10); 0 disables")
	flag.Parse()

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	// Only /api/check is rate-limited; static assets (the SPA's html/js/css) are
	// cheap to serve and limiting them would throttle ordinary page loads.
	var checkHandler http.Handler = http.HandlerFunc(handleCheck)
	if *rateLimit > 0 {
		burst := *rateLimit * 5
		if burst < 10 {
			burst = 10
		}
		checkHandler = newRateLimiter(*rateLimit, burst).middleware(checkHandler)
		log.Printf("rate-limiting /api/check to %g req/s per IP (burst %g)", *rateLimit, burst)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.Handle("/api/check", checkHandler)

	if *tlsDomain == "" {
		log.Printf("handlecheckerweb listening on %s (plain HTTP)", *addr)
		if err := http.ListenAndServe(*addr, mux); err != nil {
			log.Fatal(err)
		}
		return
	}

	serveTLS(mux, splitDomains(*tlsDomain), *tlsEmail, *tlsCache, *httpAddr, *httpsAddr, *tlsStaging)
}

// serveTLS runs the app over HTTPS, obtaining and auto-renewing certificates for
// domains from Let's Encrypt via the ACME protocol (golang.org/x/crypto's
// autocert). A second listener on httpAddr answers the ACME HTTP-01 challenges
// and redirects all other traffic to HTTPS. This blocks for the process
// lifetime. Requirements: the host must be reachable from the internet on both
// ports, and each domain's DNS must resolve to it.
func serveTLS(h http.Handler, domains []string, email, cacheDir, httpAddr, httpsAddr string, staging bool) {
	if len(domains) == 0 {
		log.Fatal("tls: -tls-domain must name at least one domain")
	}
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domains...),
		Cache:      autocert.DirCache(cacheDir),
		Email:      email,
	}
	if staging {
		m.Client = &acme.Client{DirectoryURL: letsEncryptStagingURL}
		log.Print("tls: using Let's Encrypt STAGING environment (certificates will be untrusted)")
	}

	// HTTP listener: serves the ACME HTTP-01 challenge responses and redirects
	// everything else to HTTPS. Without it, autocert still works via the
	// TLS-ALPN-01 challenge on 443, but plain-HTTP visitors would get nothing.
	httpSrv := &http.Server{
		Addr:              httpAddr,
		Handler:           m.HTTPHandler(nil),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("ACME / HTTP→HTTPS redirect listener on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http listener: %v", err)
		}
	}()

	httpsSrv := &http.Server{
		Addr:              httpsAddr,
		Handler:           h,
		TLSConfig:         m.TLSConfig(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("handlecheckerweb serving HTTPS for %s on %s (certs cached in %q)",
		strings.Join(domains, ", "), httpsAddr, cacheDir)
	// Certs come from the autocert manager via TLSConfig.GetCertificate, so the
	// cert/key file arguments are empty.
	if err := httpsSrv.ListenAndServeTLS("", ""); err != nil {
		log.Fatal(err)
	}
}

// splitDomains parses a comma-separated domain list into trimmed, non-empty
// names.
func splitDomains(s string) []string {
	var out []string
	for _, d := range strings.Split(s, ",") {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// checkRequest is the body of POST /api/check.
type checkRequest struct {
	Candidate string   `json:"candidate"`
	Reserved  []string `json:"reserved"`
	Existing  []string `json:"existing"`
}

// issueDTO is one finding, shaped for the client. Presentation (color, labels)
// is left to the client; the server returns data only.
type issueDTO struct {
	Severity     string `json:"severity"`     // "HIGH", "MEDIUM", ...
	SeverityRank int    `json:"severityRank"` // numeric, higher = worse
	Kind         string `json:"kind"`
	Detail       string `json:"detail"`
	B            string `json:"b"`      // conflicting baseline term ("" for self checks)
	Source       string `json:"source"` // "reserved" | "existing" | "self"
}

// checkResponse is the body returned by POST /api/check.
type checkResponse struct {
	Candidate string     `json:"candidate"`
	Issues    []issueDTO `json:"issues"`
	Worst     string     `json:"worst"`     // severity string of the worst issue, "" if none
	WorstRank int        `json:"worstRank"` // -1 if no issues
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req checkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Candidate) == "" {
		http.Error(w, "candidate is required", http.StatusBadRequest)
		return
	}

	// Membership sets for tagging each conflict's source. Reserved wins ties.
	reservedSet := make(map[string]bool, len(req.Reserved))
	for _, t := range req.Reserved {
		reservedSet[t] = true
	}

	baseline := make([]string, 0, len(req.Reserved)+len(req.Existing))
	baseline = append(baseline, req.Reserved...)
	baseline = append(baseline, req.Existing...)

	issues := checker.CheckAgainst(req.Candidate, baseline)

	resp := checkResponse{
		Candidate: req.Candidate,
		Issues:    make([]issueDTO, 0, len(issues)),
		WorstRank: -1,
	}
	for _, is := range issues {
		source := "self"
		if is.B != "" {
			if reservedSet[is.B] {
				source = "reserved"
			} else {
				source = "existing"
			}
		}
		rank := int(is.Severity)
		resp.Issues = append(resp.Issues, issueDTO{
			Severity:     is.Severity.String(),
			SeverityRank: rank,
			Kind:         is.Kind,
			Detail:       is.Detail,
			B:            is.B,
			Source:       source,
		})
		if rank > resp.WorstRank {
			resp.WorstRank = rank
			resp.Worst = is.Severity.String()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encode: %v", err)
	}
}
