package checker

import (
	"fmt"
	"strings"
)

// safetyWords are emergency / distress words. A callsign that is or sounds like
// one is CRITICAL: heard out of context on a live net it can be mistaken for a
// real call for help, or be misheard during an actual incident. Includes the
// three international voice-distress signals (mayday, pan-pan, securité).
var safetyWords = []string{
	"mayday", "panpan", "securite",
	"help", "medic", "emergency", "evac", "rescue",
}

// prowords are radio procedure words reserved for running the net. A callsign
// that is or sounds like one is HIGH: it will be parsed as procedure rather than
// identity ("GoldBreak" heard as the proword "break").
var prowords = []string{
	"roger", "copy", "wilco", "affirmative", "affirm", "negative",
	"disregard", "standby", "correction", "break", "over", "out",
}

// substringProwords are the proword/safety words distinctive enough to also be
// matched as a substring of the whole, glued handle — not only as a spoken word
// token. Tokenization breaks only on camelCase and separator boundaries, so
// without this "BreakBreak" (two tokens) would be caught while "Breakbreak" or
// "BREAKBREAK" (one glued token) would not; detection must not depend on a
// handle's capitalization.
//
// The short, ubiquitous prowords ("over", "out") and the common-English safety
// words ("help", "medic") are deliberately excluded — as substrings they would
// fire on "Rover", "Scout", "Helper", "Comedic" and the like, so they stay
// token/sound-only.
var substringProwords = map[string]bool{
	// distinctive prowords
	"roger": true, "copy": true, "wilco": true, "affirmative": true,
	"affirm": true, "negative": true, "disregard": true, "standby": true,
	"correction": true, "break": true,
	// distinctive safety / distress words
	"mayday": true, "panpan": true, "securite": true, "emergency": true,
	"evac": true, "rescue": true,
}

// prowordAllowlist holds common words that embed a substring-matched proword but
// are innocent (cf. the Scunthorpe problem). Matched against the whole
// normalized handle.
var prowordAllowlist = map[string]bool{
	"breakfast": true, "daybreak": true, "heartbreak": true, "outbreak": true,
}

// checkProwords flags a callsign that is, or sounds like, a radio procedure word
// (HIGH) or an emergency word (CRITICAL). It matches each spoken word token of
// the callsign — so a proword buried in a compound handle is caught across the
// camelCase boundary — both exactly and by ear (catching respellings like
// "Brake" for "break"), and additionally matches the distinctive words as a
// substring of the glued handle so detection is independent of capitalization.
// It returns at most one issue, safety words first.
func checkProwords(c string) []Issue {
	spoken := expandDigits(c)
	forms := tokens(spoken)
	norm := normalize(spoken)

	if w, ok := matchWordList(forms, norm, safetyWords); ok {
		return []Issue{{
			A: c, Severity: SevCritical, Kind: "safety-word",
			Detail: fmt.Sprintf("is or sounds like the emergency word %q — it could be misheard as a real call for help on the net", w),
		}}
	}
	if w, ok := matchWordList(forms, norm, prowords); ok {
		return []Issue{{
			A: c, Severity: SevHigh, Kind: "proword",
			Detail: fmt.Sprintf("is or sounds like the radio procedure word %q, which is reserved for net procedure and will be confused with it", w),
		}}
	}
	return nil
}

// matchWordList reports the first word in list that the callsign matches: either
// a spoken word token equals or sounds like it (so "Over" and respellings like
// "Brake" are caught), or — for the distinctive substringProwords — it appears
// as a substring of the glued, normalized handle, so case-glued compounds like
// "Breakbreak" are caught however they are capitalized. The prowordAllowlist
// exempts innocent embeddings ("Breakfast"). Tokens shorter than 3 letters are
// skipped as too noisy.
func matchWordList(forms []string, norm string, list []string) (string, bool) {
	allowed := prowordAllowlist[norm]
	for _, w := range list {
		for _, form := range forms {
			if len(form) < 3 {
				continue
			}
			if form == w || soundsLikeWord(form, w) {
				return w, true
			}
		}
		if !allowed && substringProwords[w] && strings.Contains(norm, w) {
			return w, true
		}
	}
	return "", false
}
