# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A CLI that vets proposed Burning Man radio callsigns for confusability — by
spelling, by sight, and especially by how they sound on the air. It takes a list
of candidate callsigns, checks each one alone and every pair against each other,
and prints ranked findings.

A companion web interface (`cmd/handlecheckerweb`) wraps the same engine in an
incremental intake workflow: seed reserved handles and existing handles, then
review proposed handles one at a time against that baseline and approve/reject
each — approvals join the baseline for subsequent checks.

## Commands

```sh
# Run against a list of callsigns
go run ./cmd/handlecheckercli GoldWing GoldBar golffoxtrot Knight Nite Echo

# Run the web interface, then open http://localhost:8080
go run ./cmd/handlecheckerweb            # --addr :8080 by default

# Build a binary
go build -o handlecheckercli ./cmd/handlecheckercli

# Run all tests
go test ./...

# Run a single package's tests / one test
go test ./internal/checker
go test ./internal/phonetic -run TestRhyme -v

# Build the Docker image (bundles espeak-ng — see below; builds both binaries)
docker build -t handlechecker .

# Run the CLI in Docker (the image's default entrypoint)
docker run --rm handlechecker GoldWing Knight Nite

# Run the web interface in Docker, then open http://localhost:8080.
# Override the entrypoint to switch from the default CLI to the web server.
docker run --rm --entrypoint handlecheckerweb -p 8080:8080 handlechecker

# Serve the web interface publicly with auto-provisioned Let's Encrypt TLS.
# The host must be internet-reachable on ports 80+443 and DNS for the domain
# must point at it. Persist -tls-cache so certs survive restarts (rate limits!).
docker run --rm --entrypoint handlecheckerweb -p 80:80 -p 443:443 \
  -v handlechecker-certs:/certs handlechecker \
  --tls-domain handles.example.org --tls-email you@example.org --tls-cache /certs

# Or via docker-compose: set TLS_DOMAIN/TLS_EMAIL in .env (see .env.example),
# then bring it up. Maps ./certs on the host as the TLS cache; exposes 80+443.
cp .env.example .env && $EDITOR .env
docker compose up -d --build
```

Running in Docker (rather than `go run`) is also how you get the espeak-ng
phoneme engine, which the local PATH usually lacks — see the engine notes below.

Web server flags: `--addr` (plain-HTTP listen address, default `:8080`, used when
`--tls-domain` is empty). For public HTTPS the server speaks ACME itself (via
`golang.org/x/crypto/acme/autocert`) — no certbot: `--tls-domain` (comma-separated
domains; setting it enables HTTPS), `--tls-email`, `--tls-cache` (cert/account
cache dir, default `certs` — **persist this** so renewals and restarts don't
re-hit Let's Encrypt rate limits), `--https-addr` (default `:443`), `--http-addr`
(default `:80`, serves the ACME HTTP-01 challenge and redirects to HTTPS), and
`--tls-staging` (use the Let's Encrypt staging CA while testing). Certificates
renew automatically in the background.

Access control: set the `ACCESS_KEYS` env var (comma-separated secrets) to gate
the whole site — every request, page and API alike, must present a valid key.
Unset/empty leaves it open (the local/dev default). A visitor may supply the key
via `?key=SECRET` (cached afterward in an HttpOnly `hc_access` cookie and
scrubbed from the URL on the next navigation), an `X-Access-Key` header, or HTTP
Basic Auth (the key is the password; username ignored). Keys are compared in
constant time. An unauthorized **browser navigation** gets a styled in-app
"enter access key" page (self-contained, since the real CSS is itself gated)
whose form submits `?key=`; **API/fetch callers** (and anything under `/api/`)
get a bare `401` with `WWW-Authenticate: Basic` instead. There are no per-user
accounts — it's a shared bouncer, not identity. Wrong guesses are throttled
per client IP (a token bucket reusing `ratelimit.go`: a short burst, then
~1 guess per few seconds, `429` + `Retry-After` once spent); a **correct** key
never touches the limiter, and a bare page view (no key presented) costs no
budget, so only actual guesses are rate-limited. See `auth.go`.

CLI flags: `--min` (minimum severity to print, default `info`), `--fail-on`
(exit non-zero at this severity or above, default `high`, `never` to always exit
0), `--no-color`.

## Architecture

Packages with a one-way dependency `cmd → checker → phonetic`; both `cmd`
binaries consume `checker`:

- **`cmd/handlecheckercli`** — flag parsing, severity parsing, and all terminal
  presentation (ANSI color, exit codes). No analysis logic lives here.

- **`cmd/handlecheckerweb`** — a stateless HTTP server (`main.go`) plus an
  embedded vanilla-JS single page (`static/`, served via `go:embed`). The
  browser holds all the lists (persisted to `localStorage`) and re-sends them
  per check; `POST /api/check` calls `checker.CheckAgainst` and returns JSON
  findings tagged by source (`reserved`/`existing`/`self`). The only server-side
  state is the in-process phoneme cache, which stays warm for the process
  lifetime. No analysis logic lives here either. `auth.go` adds an optional
  shared-key gate (`ACCESS_KEYS` env var) as middleware wrapping the whole mux;
  empty means no gate. The unauthorized-browser page is a self-contained Go
  template embedded from `access.html` (kept out of `static/`, which the file
  server would otherwise expose and the gate would block).

- **`internal/checker`** — the analysis engine. `Analyze` runs `checkSingle` on
  each callsign and `checkPair` on every unordered pair, returning `[]Issue`
  sorted most-severe-first; `CheckAgainst(candidate, baseline)` is the
  one-vs-many variant the web app uses (candidate alone plus candidate against
  each baseline term, never baseline-vs-baseline; candidate is always `A`). Every finding is an `Issue{A, B, Severity, Kind,
  Detail}` — `B` empty for single-callsign findings. Severities are an ordered
  enum `SevInfo < SevLow < SevMedium < SevHigh < SevCritical`. Spelling/sight
  helpers live alongside: `nato.go` (decompose a name into NATO alphabet words),
  `written.go` (homoglyph folding for written-roster look-alikes), `digits.go`
  (`expandDigits` reads a digit as its spoken word, "Dog4" -> "DogFour"), the
  `levenshtein`/`tokens` helpers in `checker.go`, `profanity.go`
  (`checkProfanity`: a per-callsign CRITICAL check for callsigns that contain or
  sound like a swear word, with a Scunthorpe-style allowlist), and `prowords.go`
  (`checkProwords`: a per-callsign check flagging callsigns that are or sound
  like a radio procedure word — HIGH — or an emergency word — CRITICAL; matched
  per spoken token, with the distinctive words also matched as a substring of
  the glued handle so detection is independent of capitalization, e.g.
  "Breakbreak"). `checkSingle`/`checkPair` run
  the sound- and spelling-based checks on the digit-expanded form but keep the
  raw form for the written-roster checks (`look-alike`, `confusable-chars`),
  where the digit glyph itself is the concern.

- **`internal/phonetic`** — all sound and prosody comparison. This is the heart
  of the tool and has **two interchangeable sound engines**:
  1. **Phoneme distance (preferred)** — `phonemes.go` + `features.go`. Shells out
     to `espeak-ng` to get a real phoneme sequence (vowel quality included), then
     computes a feature-weighted edit distance over articulatory features
     (`artic` in `features.go`). `PhoneticDistance` returns a normalized distance
     and `ok=false` when espeak-ng is unavailable.
  2. **Metaphone 3 (fallback)** — `metaphone3.go`. Pure Go via
     `github.com/dlclark/metaphone3`. `SoundsAlike`/`SoundsSimilar`/
     `SoundsLikeStartOf` cross-match primary+secondary keys with and without
     vowel positions. Metaphone collapses *all* vowels to one value, so it cannot
     tell `Gold` from `Gild` — which is exactly why engine 1 is preferred.

  `prosody.go` adds `Rhyme` (final-vowel rime) and `SyllableCount`. Each derives
  its answer from the espeak-ng phoneme stream when available (so silent letters
  and digraphs are handled correctly) and falls back to a spelling heuristic
  otherwise — including when a phoneme token is unrecognized, so a partial
  reading is never trusted. Their rime keys are opaque and engine-dependent;
  callers compare them only for equality.

### Two things to know before changing the engines

- **The espeak-ng dependency is optional and runtime-only.** It is *not* on the
  developer's PATH by default (only the Docker image bundles it), so locally the
  Metaphone fallback path runs. `checkPair` in `checker.go` calls
  `phonetic.PhoneticDistance` first and branches to the Metaphone functions only
  when `ok` is false — keep both branches in sync when changing sound logic, and
  test both with and without espeak-ng installed.

- **Severity thresholds are tuned constants, not arbitrary.** The phoneme
  HIGH/MED cutoffs (`phonemeHighMax`, `phonemeMedMax` in `checker.go`) and the
  feature weights / `vowelWeight` in `phonemes.go`/`features.go` were hand-tuned
  against a battery of real pronunciations (see comments citing Gold/Cold=0.02,
  Gold/Gild=0.13). Changing them shifts which pairs get flagged HIGH vs MEDIUM —
  re-validate against the test battery in `phonemes_test.go`.

### Avoiding duplicate findings

`checkPair` deliberately suppresses weaker findings already explained by a
stronger one: a reported rhyme suppresses the raw common-suffix finding, a strong
phonetic match (`strongSound`) suppresses rhyme/suffix, and a shared whole-word
token (`explainsAffix`) suppresses the common-prefix/suffix findings. Preserve
this layering when adding checks so the output stays non-redundant.

## Requirements

Go 1.26+. One module dependency (`dlclark/metaphone3`). `espeak-ng` is an
optional runtime binary that upgrades the sound check; absent, the tool degrades
gracefully.

## Roadmap

See `TODO.md` — the main open item is a profanity / unfortunate-sounds check
(per-callsign, catching swear words across camelCase boundaries and phonetic
spellings, with a Scunthorpe-style allowlist).
