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
//	handlecheckerweb --addr :8080
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/srabraham/handlechecker/internal/checker"
)

//go:embed static
var staticFS embed.FS

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/check", handleCheck)

	log.Printf("handlecheckerweb listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
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
