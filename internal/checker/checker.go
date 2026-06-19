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

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity > issues[j].Severity
		}
		if issues[i].A != issues[j].A {
			return issues[i].A < issues[j].A
		}
		return issues[i].B < issues[j].B
	})
	return issues
}

// --- single-callsign checks --------------------------------------------------

func checkSingle(c string) []Issue {
	var issues []Issue
	norm := normalize(c)

	if norm == "" {
		issues = append(issues, Issue{
			A: c, Severity: SevHigh, Kind: "empty",
			Detail: "callsign has no letters",
		})
		return issues
	}

	if words, ok := natoDecompose(norm); ok {
		switch {
		case len(words) >= 2:
			issues = append(issues, Issue{
				A: c, Severity: SevHigh, Kind: "nato-concatenation",
				Detail: fmt.Sprintf("is just NATO phonetic words strung together (%s)", strings.Join(words, " + ")),
			})
		case len(words) == 1:
			issues = append(issues, Issue{
				A: c, Severity: SevMedium, Kind: "nato-word",
				Detail: fmt.Sprintf("is itself a NATO phonetic word (%q)", words[0]),
			})
		}
	} else if lead := leadingNatoWord(norm); lead != "" && len(lead) >= 4 {
		issues = append(issues, Issue{
			A: c, Severity: SevLow, Kind: "nato-prefix",
			Detail: fmt.Sprintf("starts with the NATO phonetic word %q", lead),
		})
	}

	switch {
	case len(norm) < 3:
		issues = append(issues, Issue{
			A: c, Severity: SevMedium, Kind: "too-short",
			Detail: fmt.Sprintf("is very short (%d letters); short callsigns are easily lost in noise", len(norm)),
		})
	case len(norm) > 14:
		issues = append(issues, Issue{
			A: c, Severity: SevLow, Kind: "too-long",
			Detail: fmt.Sprintf("is long (%d letters); long callsigns are slow and error-prone on the air", len(norm)),
		})
	}

	if hasConfusableChars(c) {
		issues = append(issues, Issue{
			A: c, Severity: SevLow, Kind: "confusable-chars",
			Detail: "contains characters easily misread on a written roster (e.g. '0' vs 'O', '1' vs 'l')",
		})
	}

	return issues
}

// --- pairwise checks ---------------------------------------------------------

func checkPair(a, b string) []Issue {
	var issues []Issue
	na, nb := normalize(a), normalize(b)
	if na == "" || nb == "" {
		return issues // empty handled by single check
	}

	add := func(sev Severity, kind, detail string) {
		issues = append(issues, Issue{A: a, B: b, Severity: sev, Kind: kind, Detail: detail})
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

	// One contained in the other.
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
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
	shared := sharedTokens(a, b)
	for _, t := range shared {
		add(SevMedium, "shared-word", fmt.Sprintf("both contain the word %q", t))
	}

	// Shared leading/trailing letters (only when not already explained by a
	// shared whole word).
	if pre := commonPrefixLen(na, nb); pre >= 3 && !explainsAffix(shared, na[:pre]) {
		add(SevMedium, "common-prefix",
			fmt.Sprintf("start with the same %d letters (%q); they sound alike on first syllable", pre, na[:pre]))
	}
	// Phonetic: the most important radio concern — sounding the same. Double
	// Metaphone cross-matches primary/secondary pronunciations.
	soundAlike := phonetic.SoundsAlike(a, b)
	switch {
	case soundAlike:
		add(SevHigh, "sound-alike", "sound nearly identical (matching Double Metaphone code)")
	case phonetic.Soundex(a) == phonetic.Soundex(b):
		add(SevMedium, "sound-similar", "have the same Soundex code; likely confusable by ear")
	case phonetic.SoundsLikeStartOf(a, b):
		add(SevLow, "sound-prefix", "one sounds like the start of the other")
	}

	// Rhyme: same final vowel sound. Takes precedence over a raw common suffix,
	// which is usually just describing the same rhyme.
	ra, rb := phonetic.Rhyme(a), phonetic.Rhyme(b)
	rhyme := !soundAlike && ra != "" && ra == rb && len(ra) >= 2
	if rhyme {
		add(SevLow, "rhyme", fmt.Sprintf("rhyme (both end with the %q sound)", ra))
	}

	// Shared trailing letters, when not already explained by a shared word or
	// by a reported rhyme.
	if suf := commonSuffixLen(na, nb); suf >= 3 && !rhyme && !explainsAffix(shared, na[len(na)-suf:]) {
		add(SevLow, "common-suffix",
			fmt.Sprintf("end with the same %d letters (%q)", suf, na[len(na)-suf:]))
	}

	// Cadence: same number of syllables. Only noted for longer callsigns, where
	// matching rhythm is more distinctive (and short matches are too common).
	if sa, sb := phonetic.SyllableCount(a), phonetic.SyllableCount(b); sa == sb && sa >= 3 {
		add(SevInfo, "syllable-count",
			fmt.Sprintf("have the same syllable count (%d), giving a similar cadence on the air", sa))
	}

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
