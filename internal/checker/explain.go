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

	// --- sound: containment, then phoneme distance + overlap, then Metaphone 3 --
	strongSound := false
	phonemeSev := Severity(-1)

	// Phonetic containment — the spoken analogue of the spelled "substring" check.
	if cdist, ok := phonetic.PhoneticContainment(sa, sb); !ok {
		add("phonetic containment", false, 0, "espeak-ng unavailable")
	} else if cdist <= containMaxDist && !substr {
		add("phonetic containment", true, SevHigh,
			fmt.Sprintf("one sounds like the whole of the other (edge distance %.2f <= %.2f)", cdist, containMaxDist))
		phonemeSev = SevHigh
	} else if cdist <= containMaxDist {
		add("phonetic containment", false, 0,
			fmt.Sprintf("one sounds contained in the other (edge distance %.2f), but the spelled substring check already says so", cdist))
	} else {
		add("phonetic containment", false, 0,
			fmt.Sprintf("neither pronunciation is heard whole at an edge of the other (best edge distance %.2f > %.2f)", cdist, containMaxDist))
	}

	d, dok := phonetic.PhoneticDistance(sa, sb)
	ovSyl, ovDist, ovOK := phonetic.PhoneticOverlap(sa, sb)
	switch {
	case !dok:
		add("phoneme distance (espeak-ng)", false, 0, "espeak-ng unavailable — sound check falls back to Metaphone 3")
		add("shared sound run", false, 0, "espeak-ng unavailable")
	case phonemeSev >= SevHigh:
		// Containment already flagged the pair; checkPair skips the rest.
		add("phoneme distance (espeak-ng)", false, 0,
			fmt.Sprintf("distance %.2f — moot, phonetic containment already flagged the pair", d))
		add("shared sound run", false, 0,
			fmt.Sprintf("best run %d syllable(s) at distance %.2f — moot, containment already flagged the pair", ovSyl, ovDist))
	default:
		switch {
		case d <= phonemeHighMax:
			add("phoneme distance (espeak-ng)", true, SevHigh,
				fmt.Sprintf("distance %.2f <= %.2f — sound nearly identical", d, phonemeHighMax))
			phonemeSev = SevHigh
		case d <= phonemeMedMax && ovOK && ovDist <= similarOverlapMax:
			add("phoneme distance (espeak-ng)", true, SevMedium,
				fmt.Sprintf("distance %.2f <= %.2f with a clean shared run (overlap %.2f <= %.2f) — sound similar",
					d, phonemeMedMax, ovDist, similarOverlapMax))
			phonemeSev = SevMedium
		case d <= phonemeMedMax:
			add("phoneme distance (espeak-ng)", false, 0,
				fmt.Sprintf("distance %.2f <= %.2f but no clean shared run (overlap %.2f > %.2f) — coincidental, not flagged",
					d, phonemeMedMax, ovDist, similarOverlapMax))
		default:
			add("phoneme distance (espeak-ng)", false, 0,
				fmt.Sprintf("distance %.2f > %.2f — too far apart", d, phonemeMedMax))
		}

		// Shared multi-syllable run. checkPair only considers it when the global
		// distance did not already flag the pair.
		switch {
		case phonemeSev >= SevMedium:
			add("shared sound run", false, 0,
				fmt.Sprintf("best run %d syllable(s) at distance %.2f — moot, the phoneme distance already flagged the pair", ovSyl, ovDist))
		case ovOK && ovSyl >= overlapMinSyllables && ovDist <= overlapMaxDist:
			add("shared sound run", true, SevMedium,
				fmt.Sprintf("share a %d-syllable run of sounds (distance %.2f <= %.2f)", ovSyl, ovDist, overlapMaxDist))
			phonemeSev = SevMedium
		default:
			add("shared sound run", false, 0,
				fmt.Sprintf("best run %d syllable(s) at distance %.2f (need >= %d syllables and <= %.2f)",
					ovSyl, ovDist, overlapMinSyllables, overlapMaxDist))
		}
	}
	if phonemeSev >= SevMedium {
		strongSound = true
	}

	// Metaphone 3, surfaced only when it warns more strongly than the phoneme verdict.
	if msev, mkind, mdetail, mok := metaphoneSound(sa, sb); mok && msev > phonemeSev {
		add("Metaphone 3", true, msev, mdetail)
		if msev >= SevMedium {
			strongSound = true
		}
	} else if mok {
		add("Metaphone 3", false, 0, fmt.Sprintf("%s, but the phoneme engine already rates the pair at least as strongly", mkind))
	} else {
		add("Metaphone 3", false, 0, "no matching consonant skeleton")
	}

	// Rhyme, and its promotion to sound-similar when the openings also match.
	ra, rb := phonetic.Rhyme(sa), phonetic.Rhyme(sb)
	rimesMatch := ra != "" && ra == rb && len(ra) >= 2
	od, ook := phonetic.SharedOpening(sa, sb)
	rhymeFired := rimesMatch && !strongSound
	switch {
	case rimesMatch && strongSound:
		add("rhyme", false, 0, fmt.Sprintf("both end with the %q sound, but suppressed — a stronger sound finding already explains the pair", ra))
	case rhymeFired && ook && od <= openingMaxDist:
		add("rhyme + shared opening", true, SevMedium,
			fmt.Sprintf("open alike (onset distance %.2f <= %.2f) and rhyme (both end with the %q sound) — alike at both ends", od, openingMaxDist, ra))
	case rhymeFired && ook:
		add("rhyme", true, SevLow,
			fmt.Sprintf("both end with the %q sound; openings differ (onset distance %.2f > %.2f), so it stays a plain rhyme", ra, od, openingMaxDist))
	case rhymeFired:
		add("rhyme", true, SevLow, fmt.Sprintf("both end with the %q sound (opening comparison needs espeak-ng)", ra))
	case ra == "" || rb == "":
		add("rhyme", false, 0, "could not determine a rime for one callsign")
	default:
		add("rhyme", false, 0, fmt.Sprintf("rimes differ (%q vs %q)", ra, rb))
	}

	// Shared ending letters (only when not already explained by a rhyme, a strong
	// sound match, or a shared whole word).
	switch suf := commonSuffixLen(na, nb); {
	case suf >= 3 && !rhymeFired && !strongSound && !explainsAffix(shared, na[len(na)-suf:]):
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
