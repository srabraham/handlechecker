# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A CLI that vets proposed Burning Man radio callsigns for confusability — by
spelling, by sight, and especially by how they sound on the air. It takes a list
of candidate callsigns, checks each one alone and every pair against each other,
and prints ranked findings.

## Commands

```sh
# Run against a list of callsigns
go run ./cmd/handlecheckercli GoldWing GoldBar golffoxtrot Knight Nite Echo

# Build a binary
go build -o handlecheckercli ./cmd/handlecheckercli

# Run all tests
go test ./...

# Run a single package's tests / one test
go test ./internal/checker
go test ./internal/phonetic -run TestRhyme -v

# Build the Docker image (bundles espeak-ng — see below)
docker build -t handlechecker .
docker run --rm handlechecker GoldWing Knight Nite
```

CLI flags: `--min` (minimum severity to print, default `info`), `--fail-on`
(exit non-zero at this severity or above, default `high`, `never` to always exit
0), `--no-color`.

## Architecture

Three packages, with a one-way dependency `cmd → checker → phonetic`:

- **`cmd/handlecheckercli`** — flag parsing, severity parsing, and all terminal
  presentation (ANSI color, exit codes). No analysis logic lives here.

- **`internal/checker`** — the analysis engine. `Analyze` runs `checkSingle` on
  each callsign and `checkPair` on every unordered pair, returning `[]Issue`
  sorted most-severe-first. Every finding is an `Issue{A, B, Severity, Kind,
  Detail}` — `B` empty for single-callsign findings. Severities are an ordered
  enum `SevInfo < SevLow < SevMedium < SevHigh < SevCritical`. Spelling/sight
  helpers live alongside: `nato.go` (decompose a name into NATO alphabet words),
  `written.go` (homoglyph folding for written-roster look-alikes), and the
  `levenshtein`/`tokens` helpers in `checker.go`.

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

  `prosody.go` adds `Rhyme` (final-vowel rime) and `SyllableCount`, used
  independently of the sound engine.

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
