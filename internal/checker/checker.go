// Package checker analyzes a set of proposed radio callsigns and reports ways
// in which they may be confusable with one another — visually, by spelling,
// or (most importantly) by how they sound when spoken over the radio.
package checker

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/srabraham/handlechecker/internal/phonetic"
)

// Severity ranks how concerning an issue is.
type Severity int

const (
	SevInfo Severity = iota
	SevLow
	SevMedium
	SevHigh
	SevCritical
)

func (s Severity) String() string {
	switch s {
	case SevInfo:
		return "INFO"
	case SevLow:
		return "LOW"
	case SevMedium:
		return "MEDIUM"
	case SevHigh:
		return "HIGH"
	case SevCritical:
		return "CRITICAL"
	}
	return "UNKNOWN"
}

// Issue is a single finding. For pairwise findings both A and B are set; for
// findings about a single callsign, B is empty.
type Issue struct {
	A, B     string
	Severity Severity
	Kind     string
	Detail   string
	// Score ranks how problematic this finding is relative to others at the same
	// severity (higher = worse); it is the secondary sort key within a severity
	// band. Sound findings set it to their combined confusability score (see
	// score.go) so the closest-sounding conflicts rise to the top of their band;
	// findings without a continuous metric leave it at 0 and keep their existing
	// relative order.
	Score float64
}

// Analyze runs every check over the callsigns and returns all issues found,
// sorted most-severe first.
func Analyze(callsigns []string) []Issue {
	var issues []Issue

	for _, c := range callsigns {
		issues = append(issues, checkSingle(c)...)
	}
	for i := 0; i < len(callsigns); i++ {
		for j := i + 1; j < len(callsigns); j++ {
			issues = append(issues, checkPair(callsigns[i], callsigns[j])...)
		}
	}

	sortIssues(issues)
	return issues
}

// CheckAgainst runs the single-callsign checks on candidate plus the pairwise
// checks of candidate against every term in baseline, returning all issues
// sorted most-severe first. Unlike Analyze it does not compare baseline terms
// to one another — it answers "is this one new callsign OK against the existing
// set?". In every pairwise issue the candidate is A and the baseline term is B.
func CheckAgainst(candidate string, baseline []string) []Issue {
	var issues []Issue
	issues = append(issues, checkSingle(candidate)...)
	for _, b := range baseline {
		issues = append(issues, checkPair(candidate, b)...)
	}
	sortIssues(issues)
	return issues
}

// sortIssues orders issues most-severe first, then most-problematic first within
// a severity band (by Score), then by A and B for stability.
func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity > issues[j].Severity
		}
		if issues[i].Score != issues[j].Score {
			return issues[i].Score > issues[j].Score
		}
		if issues[i].A != issues[j].A {
			return issues[i].A < issues[j].A
		}
		return issues[i].B < issues[j].B
	})
}

// --- single-callsign checks --------------------------------------------------

func checkSingle(c string) []Issue {
	var issues []Issue
	// Analyze the callsign as it is spoken — initialisms spelled out and digits
	// read as words ("K9" -> "KayNine"). The written-roster checks below still
	// use the raw form c.
	spoken := spokenForm(c)
	norm := normalize(spoken)

	if norm == "" {
		issues = append(issues, Issue{
			A: c, Severity: SevHigh, Kind: "empty",
			Detail: "callsign has no letters",
		})
		return issues
	}

	// Syllable count: aim for 2–5 so the callsign is neither too curt nor too
	// long-winded on the air.
	switch syl := phonetic.SyllableCount(spoken); {
	case syl < 2:
		issues = append(issues, Issue{
			A: c, Severity: SevMedium, Kind: "too-few-syllables",
			Detail: fmt.Sprintf("has only %d syllable; aim for 2–5 so it isn't too short on the air", syl),
		})
	case syl > 5:
		issues = append(issues, Issue{
			A: c, Severity: SevLow, Kind: "too-many-syllables",
			Detail: fmt.Sprintf("has %d syllables; aim for 2–5 so it isn't too long-winded on the air", syl),
		})
	}

	if hasConfusableChars(c) {
		issues = append(issues, Issue{
			A: c, Severity: SevLow, Kind: "confusable-chars",
			Detail: "contains characters easily misread on a written roster (e.g. '0' vs 'O', '1' vs 'l')",
		})
	}

	// Profanity is disqualifying on its own — flag a callsign that is, contains,
	// or sounds like a swear word (CRITICAL).
	issues = append(issues, checkProfanity(c)...)

	// Procedure words (HIGH) and emergency words (CRITICAL): a callsign that
	// sounds like net procedure or a distress call is confusable with it.
	issues = append(issues, checkProwords(c)...)

	return issues
}

// metaphoneSound returns the strongest Metaphone-3 sound finding for the pair,
// with ok=false if none. Buckets mirror the phoneme-distance ones: a matching
// key (with or without vowel positions) is "sound-alike" (HIGH), a shared
// consonant skeleton is "sound-similar" (MEDIUM), and one sounding like the
// start of the other is "sound-prefix" (LOW).
func metaphoneSound(a, b string) (sev Severity, kind, detail string, ok bool) {
	switch {
	case phonetic.SoundsAlike(a, b):
		return SevHigh, "sound-alike", "sound nearly identical (matching Metaphone 3 key, vowels included)", true
	case phonetic.SoundsSimilar(a, b):
		return SevMedium, "sound-similar", "share the same consonant sounds; likely confusable by ear", true
	case phonetic.SoundsLikeStartOf(a, b):
		return SevLow, "sound-prefix", "one sounds like the start of the other", true
	}
	return 0, "", "", false
}

// --- pairwise checks ---------------------------------------------------------

func checkPair(a, b string) []Issue {
	var issues []Issue
	// Compare the callsigns as spoken — initialisms spelled out and digits read
	// as words ("Dog4" -> "DogFour") — for every sound- and spelling-based
	// check. The written-roster look-alike check below keeps the raw forms a, b.
	sa, sb := spokenForm(a), spokenForm(b)
	na, nb := normalize(sa), normalize(sb)
	if na == "" || nb == "" {
		return issues // empty handled by single check
	}

	add := func(sev Severity, kind, detail string) {
		issues = append(issues, Issue{A: a, B: b, Severity: sev, Kind: kind, Detail: detail})
	}
	// addSound is add for the combined-score sound finding: Score carries the
	// confusability score so that, among same-severity sound findings, the
	// closest match sorts to the top.
	addSound := func(sev Severity, kind, detail string, score float64) {
		issues = append(issues, Issue{A: a, B: b, Severity: sev, Kind: kind, Detail: detail, Score: score})
	}

	// Identical (ignoring case/punctuation/spacing).
	if na == nb {
		add(SevCritical, "duplicate", "are effectively identical")
		return issues // nothing else worth saying
	}

	// Look identical on a written/printed roster once confusable characters
	// (0/O, 1/l, rn/m, ...) are folded together.
	if fa, fb := homoglyphFold(a), homoglyphFold(b); fa != "" && fa == fb {
		add(SevHigh, "look-alike", "look identical on a written or printed roster (confusable characters)")
	}

	// One contained in the other (by spelling).
	substr := strings.Contains(na, nb) || strings.Contains(nb, na)
	if substr {
		add(SevHigh, "substring", "one is fully contained in the other")
	}

	// Edit distance: how easily one is mistaken for the other when written
	// down or misheard by a letter or two.
	dist := levenshtein(na, nb)
	minLen := min(len(na), len(nb))
	switch {
	case dist == 1:
		add(SevHigh, "edit-distance", "differ by only a single letter")
	case dist == 2 && minLen >= 4:
		add(SevMedium, "edit-distance", "differ by only two letters")
	}

	// Shared word tokens, e.g. "GoldWing" / "GoldBar" both contain "Gold".
	shared := sharedTokens(sa, sb)
	for _, t := range shared {
		add(SevMedium, "shared-word", fmt.Sprintf("both contain the word %q", t))
	}

	// Shared leading/trailing letters (only when not already explained by a
	// shared whole word).
	if pre := commonPrefixLen(na, nb); pre >= 3 && !explainsAffix(shared, na[:pre]) {
		add(SevMedium, "common-prefix",
			fmt.Sprintf("start with the same %d letters (%q); they sound alike on first syllable", pre, na[:pre]))
	}
	// Phonetic: the most important radio concern — sounding the same. All the
	// sound decisions are made once in evaluateSound (score.go): the combined
	// confusability score over the espeak-ng signals (global distance,
	// containment, shared run, rhyme+onset, stress contour) yields at most one
	// sound finding, and Metaphone 3 — always consulted — is surfaced only when
	// it warns more strongly, so the two engines never emit duplicate findings.
	// When espeak-ng is unavailable Metaphone is the only sound engine, plus the
	// spelling-heuristic rhyme.
	snd := evaluateSound(sa, sb, substr)
	if snd.verdictFired {
		addSound(snd.verdict.sev, snd.verdict.kind, snd.verdict.detail, snd.verdict.total)
	}
	if snd.metaphoneFired {
		add(snd.msev, snd.mkind, snd.mdetail)
	}
	if snd.fallbackRhyme {
		add(SevLow, "rhyme", fmt.Sprintf("rhyme (both end with the %q sound)", snd.rime))
	}

	// Shared trailing letters, when not already explained by a shared word, a
	// matching rime, or a strong sound match.
	if suf := commonSuffixLen(na, nb); suf >= 3 && snd.rime == "" && !snd.strongSound && !explainsAffix(shared, na[len(na)-suf:]) {
		add(SevLow, "common-suffix",
			fmt.Sprintf("end with the same %d letters (%q)", suf, na[len(na)-suf:]))
	}

	// Cadence: same number of syllables. Disabled for now (kept for later
	// consideration) — was too noisy as an INFO-level signal.
	// if sa, sb := phonetic.SyllableCount(a), phonetic.SyllableCount(b); sa == sb && sa >= 3 {
	// 	add(SevInfo, "syllable-count",
	// 		fmt.Sprintf("have the same syllable count (%d), giving a similar cadence on the air", sa))
	// }

	return issues
}

// --- string helpers ----------------------------------------------------------

// normalize lower-cases and strips everything but ASCII letters.
func normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + ('a' - 'A'))
		} else if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// tokens splits a callsign into lower-cased word tokens, breaking on camelCase
// boundaries and any non-letter separators. "GoldWing-2" -> ["gold","wing"].
//
// It also splits an acronym glued to the front of a word: when an all-uppercase
// run is immediately followed by a lowercase letter, the run's last capital is
// the onset of that word, not part of the run — "DMVGuy" is "DMV" + "Guy",
// "USBKey" is "USB" + "Key". This runs only when at least two capitals precede
// the onset, so an ambiguous single leading capital ("GBush", which might be a
// name) is left glued. This is a written-roster decomposition only — the spoken
// form leaves these forms verbatim, since espeak-ng already voices them
// correctly (see expandInitialisms).
func tokens(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	prevLower := false
	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			if prevLower { // camelCase boundary
				flush()
			}
			cur = append(cur, r)
			prevLower = false
		case unicode.IsLetter(r):
			// First lowercase letter after an all-uppercase run: peel the run's last
			// capital off as this word's onset and flush the rest as an acronym. The
			// run is all-uppercase here (any separator or camelCase boundary would
			// have flushed it), so len(cur) >= 3 means >= 2 acronym letters + 1 onset.
			if !prevLower && len(cur) >= 3 {
				onset := cur[len(cur)-1]
				cur = cur[:len(cur)-1]
				flush()
				cur = append(cur, onset)
			}
			cur = append(cur, r)
			prevLower = true
		default:
			flush()
			prevLower = false
		}
	}
	flush()
	return out
}

// sharedTokens returns word tokens (length >= 3) common to both callsigns.
func sharedTokens(a, b string) []string {
	set := make(map[string]bool)
	for _, t := range tokens(a) {
		if len(t) >= 3 {
			set[t] = true
		}
	}
	var shared []string
	seen := make(map[string]bool)
	for _, t := range tokens(b) {
		if len(t) >= 3 && set[t] && !seen[t] {
			shared = append(shared, t)
			seen[t] = true
		}
	}
	sort.Strings(shared)
	return shared
}

// explainsAffix reports whether one of the shared tokens already accounts for
// the given prefix/suffix, to avoid reporting the same overlap twice.
func explainsAffix(shared []string, affix string) bool {
	for _, t := range shared {
		if t == affix {
			return true
		}
	}
	return false
}

func commonPrefixLen(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

func commonSuffixLen(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[len(a)-1-i] == b[len(b)-1-i] {
		i++
	}
	return i
}

// levenshtein returns the edit distance between two strings.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
