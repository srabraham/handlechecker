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
	// band. Sound findings set it from their phoneme distance (1-distance) so the
	// closest-sounding conflicts rise to the top of their band; findings without a
	// continuous metric leave it at 0 and keep their existing relative order.
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

// Phoneme-distance thresholds (from espeak-ng), tuned against a battery of
// real pronunciations: at or below High the words sound nearly identical
// (e.g. Gold/Cold = 0.02), at or below Med they sound similar (e.g. Gold/Gild
// = 0.13, Blaze/Belize = 0.23).
const (
	phonemeHighMax = 0.06
	phonemeMedMax  = 0.24
)

// A shared run of sounds (local alignment) is flagged when it spans at least
// this many syllables and matches at least this cleanly (normalized distance
// within the run). This catches pairs whose global distance looks safe only
// because they differ at the edges, e.g. "DustyDog" / "ADustyLog" — whose shared
// "Dust-y-Dog"/"Dust-y-Log" run is three syllables. PhoneticOverlap additionally
// requires the run to contain a whole word on one side, so an interior-only match
// (e.g. "Abraham" / "Zebra" sharing "-bra-") does not count.
//
// The floor is 3, not 2, on purpose. A two-syllable run is too short to be a
// reliable signal here: short words that merely share a liquid+vowel skeleton
// align cleanly enough to slip under overlapMaxDist (e.g. "Tulsa" vs the "Delta"
// of "Delta Victor" at 0.07 — a global-safe pair this is the *only* check to
// flag), yet they are not confusable on the air. Tightening overlapMaxDist can't
// fix that: those false positives score *lower* (cleaner) than the genuine
// three-syllable catches. The two-syllable matches worth keeping are already
// caught by the edit-distance, substring, or global-phoneme checks, so raising
// the floor sheds the noise without losing real conflicts.
const (
	overlapMinSyllables = 3
	overlapMaxDist      = 0.12
)

// A rhyme is promoted from a bare LOW finding to a MEDIUM "sound-similar" one
// when the two callsigns also share an opening this closely (normalized onset-
// consonant distance, see phonetic.SharedOpening). Sharing both ends — opening
// and rhyme — leaves only the middle to tell them apart, which the ear easily
// misses even when the whole-word distance looks safe. The bar is near-identical:
// it admits a shared /h/ ("Hot Guy"/"HawkEye", 0.00) but not a merely close onset
// like /m/ vs /b/ ("Monsoon"/"Balloon", 0.13), which only coincidentally rhymes.
const openingMaxDist = 0.10

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
	// addSound is add for findings carrying a phoneme distance: Score is set to
	// 1-distance so that, among same-severity sound findings, the closest match
	// (smallest distance) sorts to the top.
	addSound := func(sev Severity, kind, detail string, dist float64) {
		issues = append(issues, Issue{A: a, B: b, Severity: sev, Kind: kind, Detail: detail, Score: 1 - dist})
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
	// Phonetic: the most important radio concern — sounding the same. We run two
	// engines and would rather over-warn than miss a conflict:
	//   1. Phoneme distance (espeak-ng) — precise and vowel-aware (it can tell
	//      "Gold" from "Gild"). The primary signal when espeak-ng is installed.
	//   2. Metaphone 3 — a vowel-collapsing consonant-skeleton match. Always
	//      consulted; on its own when espeak-ng is unavailable, and *alongside*
	//      the phoneme distance otherwise so a consonant clash that espeak rates
	//      as distant still surfaces.
	// phonemeSev tracks the severity the phoneme engine assigned (-1 if it stayed
	// silent or is unavailable); Metaphone is then only surfaced when it warns
	// more strongly than that, so the two engines never emit duplicate findings.
	strongSound := false
	phonemeSev := Severity(-1)
	if d, ok := phonetic.PhoneticDistance(sa, sb); ok {
		switch {
		case d <= phonemeHighMax:
			addSound(SevHigh, "sound-alike", fmt.Sprintf("sound nearly identical (phoneme distance %.2f via espeak-ng)", d), d)
			phonemeSev = SevHigh
		case d <= phonemeMedMax:
			addSound(SevMedium, "sound-similar", fmt.Sprintf("sound similar (phoneme distance %.2f via espeak-ng)", d), d)
			phonemeSev = SevMedium
		}
		// Even when the words differ overall, a shared multi-syllable run of
		// sounds (e.g. both contain "Dusty") is easily confused on the air.
		if phonemeSev < 0 {
			if syl, od, ok2 := phonetic.PhoneticOverlap(sa, sb); ok2 &&
				syl >= overlapMinSyllables && od <= overlapMaxDist {
				addSound(SevMedium, "sound-overlap", fmt.Sprintf(
					"share a %d-syllable run of sounds (distance %.2f via espeak-ng); easily confused on the air", syl, od), od)
				phonemeSev = SevMedium
			}
		}
	}
	if phonemeSev >= SevMedium {
		strongSound = true
	}
	// Metaphone 3, always consulted. Surfaced only when it warns more strongly
	// than the phoneme verdict (phonemeSev is -1 when espeak-ng is unavailable, so
	// every Metaphone finding shows in that case — the former fallback behavior).
	if msev, mkind, mdetail, mok := metaphoneSound(sa, sb); mok && msev > phonemeSev {
		add(msev, mkind, mdetail)
		if msev >= SevMedium {
			strongSound = true
		}
	}

	// Rhyme: same final vowel sound. Takes precedence over a raw common suffix,
	// which is usually just describing the same rhyme.
	ra, rb := phonetic.Rhyme(sa), phonetic.Rhyme(sb)
	rhyme := !strongSound && ra != "" && ra == rb && len(ra) >= 2
	if rhyme {
		// A rhyme paired with a matching opening (the same onset consonant) means
		// the callsigns are alike at both ends with only the middle differing —
		// confusable on the air even when the whole-word phoneme distance looks safe
		// because one carries an extra interior consonant (e.g. "Hot Guy"/"HawkEye",
		// inflated by the /g/ of "Guy"). Promote those to sound-similar (MEDIUM); a
		// rhyme with a different opening stays a bare rhyme (LOW). When espeak-ng is
		// unavailable SharedOpening reports ok=false and the finding stays LOW.
		if od, ok := phonetic.SharedOpening(sa, sb); ok && od <= openingMaxDist {
			add(SevMedium, "sound-similar", fmt.Sprintf(
				"open alike and rhyme (both end with the %q sound); easily confused on the air", ra))
		} else {
			add(SevLow, "rhyme", fmt.Sprintf("rhyme (both end with the %q sound)", ra))
		}
	}

	// Shared trailing letters, when not already explained by a shared word or
	// by a reported rhyme.
	if suf := commonSuffixLen(na, nb); suf >= 3 && !rhyme && !strongSound && !explainsAffix(shared, na[len(na)-suf:]) {
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
