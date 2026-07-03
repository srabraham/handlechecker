package checker

import (
	"fmt"
	"strings"

	"github.com/srabraham/handlechecker/internal/phonetic"
)

// CheckExplanation is one named check's verdict on a pair, for the CLI explain
// mode (handlecheckercli --explain A B). Fired reports whether the check
// contributes a finding to the normal output (Analyze/CheckAgainst); Severity is
// meaningful only when Fired. Detail explains what the check measured and
// concluded either way — crucially, why a silent check stayed silent.
type CheckExplanation struct {
	Name     string
	Fired    bool
	Severity Severity
	Detail   string
}

// ExplainPair runs every pairwise check on a and b and returns each one's
// verdict, fired or not — a diagnostic answering "why do (or don't) these two
// match?". It mirrors checkPair step for step and reuses the same threshold
// constants and suppression rules, so a Fired explanation corresponds to a real
// finding and a silent one to a real non-finding. Keep the two in sync when
// changing either; TestExplainMatchesCheckPair guards that they agree.
func ExplainPair(a, b string) []CheckExplanation {
	sa, sb := spokenForm(a), spokenForm(b)
	na, nb := normalize(sa), normalize(sb)

	var out []CheckExplanation
	add := func(name string, fired bool, sev Severity, detail string) {
		out = append(out, CheckExplanation{Name: name, Fired: fired, Severity: sev, Detail: detail})
	}

	if na == "" || nb == "" {
		add("spoken form", false, 0, "one callsign has no letters once spoken; pairwise checks skipped")
		return out
	}

	// Identical (ignoring case/punctuation/spacing). checkPair short-circuits here,
	// so nothing below it runs — mirror that.
	if na == nb {
		add("identical", true, SevCritical, fmt.Sprintf("normalized forms are identical (%q)", na))
		return out
	}
	add("identical", false, 0, fmt.Sprintf("normalized forms differ (%q vs %q)", na, nb))

	// Written look-alike (homoglyph fold) — on the raw glyphs, not the spoken form.
	if fa, fb := homoglyphFold(a), homoglyphFold(b); fa != "" && fa == fb {
		add("written look-alike", true, SevHigh, "identical after folding confusable characters (0/O, 1/l, rn/m, …)")
	} else {
		add("written look-alike", false, 0, "differ on a written roster even after folding confusable characters")
	}

	// Containment (by spelling).
	substr := strings.Contains(na, nb) || strings.Contains(nb, na)
	if substr {
		add("containment", true, SevHigh, "one spelled form is fully contained in the other")
	} else {
		add("containment", false, 0, "neither spelled form contains the other")
	}

	// Edit distance.
	dist := levenshtein(na, nb)
	minLen := min(len(na), len(nb))
	switch {
	case dist == 1:
		add("edit distance", true, SevHigh, "differ by a single letter (Levenshtein 1)")
	case dist == 2 && minLen >= 4:
		add("edit distance", true, SevMedium, "differ by two letters (Levenshtein 2)")
	default:
		add("edit distance", false, 0, fmt.Sprintf("Levenshtein %d (HIGH at 1, MEDIUM at 2 when both >= 4 letters)", dist))
	}

	// Shared whole word.
	shared := sharedTokens(sa, sb)
	if len(shared) > 0 {
		add("shared whole word", true, SevMedium, "share the word(s) "+quoteList(shared))
	} else {
		add("shared whole word", false, 0, "no whole word (>= 3 letters) in common")
	}

	// Shared opening letters (suppressed when a shared whole word already explains it).
	switch pre := commonPrefixLen(na, nb); {
	case pre >= 3 && !explainsAffix(shared, na[:pre]):
		add("shared opening letters", true, SevMedium, fmt.Sprintf("start with the same %d letters (%q)", pre, na[:pre]))
	case pre >= 3:
		add("shared opening letters", false, 0, fmt.Sprintf("share %d leading letters (%q), already explained by a shared word", pre, na[:pre]))
	default:
		add("shared opening letters", false, 0, fmt.Sprintf("share %d leading letters (need >= 3)", pre))
	}

	// --- sound: the combined confusability score, plus Metaphone 3 --------------
	// All decisions come from evaluateSound (score.go) — the same call checkPair
	// makes — so the explanation cannot drift from the real findings. The signal
	// entries below are context (never fired); the "combined sound score" entry
	// is the one that fires.
	snd := evaluateSound(sa, sb, substr)
	if !snd.espeakOK {
		add("combined sound score", false, 0, "espeak-ng unavailable — sound check falls back to Metaphone 3")
	} else {
		sig, c := snd.verdict.sig, snd.verdict.contrib
		if substr {
			add("phonetic containment", false, 0,
				"excluded — the spelled substring check already covers a contained pronunciation")
		} else if sig.contain >= 1 {
			add("phonetic containment", false, 0, "neither pronunciation is heard whole at an edge of the other")
		} else {
			add("phonetic containment", false, 0, fmt.Sprintf(
				"one heard at an edge of the other with edge distance %.2f — contributes %.2f", sig.contain, c.contain))
		}
		add("phoneme distance (espeak-ng)", false, 0, fmt.Sprintf(
			"global distance %.2f — contributes %.2f", sig.dist, c.global))
		if sig.ovDist >= 1 {
			add("shared sound run", false, 0, "no clean shared run of sounds — contributes 0.00")
		} else {
			add("shared sound run", false, 0, fmt.Sprintf(
				"best shared run spans %d syllable(s) at distance %.2f — contributes %.2f", sig.ovSyl, sig.ovDist, c.overlap))
		}
		if sig.rime == "" {
			ra, rb := phonetic.Rhyme(sa), phonetic.Rhyme(sb)
			if ra != "" && ra == rb {
				add("rhyme + opening", false, 0, fmt.Sprintf(
					"share only a bare final vowel (%q), too slight to count as a rhyme — contributes 0.00", ra))
			} else {
				add("rhyme + opening", false, 0, fmt.Sprintf("rimes differ (%q vs %q) — contributes 0.00", ra, rb))
			}
		} else if sig.onset < 1 {
			add("rhyme + opening", false, 0, fmt.Sprintf(
				"rhyme (both end with the %q sound) with onset distance %.2f — contributes %.2f", sig.rime, sig.onset, c.ends))
		} else {
			add("rhyme + opening", false, 0, fmt.Sprintf(
				"rhyme (both end with the %q sound), openings unshared — contributes %.2f", sig.rime, c.ends))
		}
		if sig.contour != "" {
			add("stress contour", false, 0, fmt.Sprintf(
				"same stress contour (%q) — contributes %.2f", sig.contour, c.contour))
		} else {
			add("stress contour", false, 0, "stress contours differ (or too short to count) — contributes 0.00")
		}

		switch {
		case snd.verdictFired:
			add("combined sound score", true, snd.verdict.sev,
				fmt.Sprintf("score %.2f (HIGH >= %.2f, MEDIUM >= %.2f) — %s", snd.verdict.total, scoreHigh, scoreMed, snd.verdict.detail))
		case snd.verdict.fired:
			// Fired on its own terms but dropped: a stronger Metaphone finding
			// explains the plain rhyme (see evaluateSound).
			add("combined sound score", false, 0, fmt.Sprintf(
				"score %.2f is a plain rhyme (LOW), suppressed — the Metaphone 3 finding below already explains the pair", snd.verdict.total))
		case snd.verdict.total >= scoreLow && snd.verdict.total < scoreMed:
			add("combined sound score", false, 0, fmt.Sprintf(
				"score %.2f is below MEDIUM (%.2f) and the pair does not rhyme, so nothing is reported", snd.verdict.total, scoreMed))
		default:
			add("combined sound score", false, 0, fmt.Sprintf(
				"score %.2f is below every band (MEDIUM %.2f, rhyme-only LOW %.2f)", snd.verdict.total, scoreMed, scoreLow))
		}
	}

	// Metaphone 3, surfaced only when it warns more strongly than the combined
	// verdict.
	switch {
	case snd.metaphoneFired:
		add("Metaphone 3", true, snd.msev, snd.mdetail)
	case snd.mok:
		add("Metaphone 3", false, 0, fmt.Sprintf("%s, but the phoneme engine already rates the pair at least as strongly", snd.mkind))
	default:
		add("Metaphone 3", false, 0, "no matching consonant skeleton")
	}

	// Rhyme on the Metaphone-fallback path (with espeak-ng the rhyme is a signal
	// inside the combined score above).
	if !snd.espeakOK {
		switch {
		case snd.fallbackRhyme:
			add("rhyme", true, SevLow, fmt.Sprintf("both end with the %q sound (spelling heuristic)", snd.rime))
		case snd.rime != "":
			add("rhyme", false, 0, fmt.Sprintf("both end with the %q sound, but suppressed — a stronger sound finding already explains the pair", snd.rime))
		default:
			add("rhyme", false, 0, "the rimes differ (spelling heuristic)")
		}
	}

	// Shared ending letters (only when not already explained by a matching rime,
	// a strong sound match, or a shared whole word).
	switch suf := commonSuffixLen(na, nb); {
	case suf >= 3 && snd.rime == "" && !snd.strongSound && !explainsAffix(shared, na[len(na)-suf:]):
		add("shared ending letters", true, SevLow, fmt.Sprintf("end with the same %d letters (%q)", suf, na[len(na)-suf:]))
	case suf >= 3:
		add("shared ending letters", false, 0,
			fmt.Sprintf("share %d trailing letters (%q), already explained by a rhyme, sound match, or shared word", suf, na[len(na)-suf:]))
	default:
		add("shared ending letters", false, 0, fmt.Sprintf("share %d trailing letters (need >= 3)", suf))
	}

	return out
}

// quoteList renders words as a comma-separated list of quoted tokens.
func quoteList(words []string) string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = fmt.Sprintf("%q", w)
	}
	return strings.Join(q, ", ")
}
