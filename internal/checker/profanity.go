package checker

import (
	"fmt"
	"strings"

	"github.com/srabraham/handlechecker/internal/phonetic"
)

// swearWords is the hardcoded profanity list a callsign must neither contain nor
// sound like. It begins with George Carlin's "seven words you can never say on
// television" and adds a handful of other unambiguous profanities and slurs.
// Matching is deliberately over-eager (substring plus sound-alike); the
// profanityAllowlist below carves back the well-known innocent collisions.
var swearWords = []string{
	// Carlin's seven dirty words.
	"shit", "piss", "fuck", "cunt", "cocksucker", "motherfucker", "tits",
	// A few more that are offensive in any context.
	"bastard", "bitch", "whore", "twat", "slut", "prick", "dick",
	"asshole", "faggot", "nigger",
}

// profanityAllowlist holds innocent words that contain a swear word as a
// substring — the "Scunthorpe problem" (Scunthorpe contains "cunt"). When a
// callsign normalizes to exactly one of these it is exempt from the profanity
// check. Keyed by the normalized (lower-case, letters-only) form.
var profanityAllowlist = map[string]bool{
	"scunthorpe": true, // contains "cunt"
	"shiitake":   true, // contains "shit"
	"shitake":    true, // contains "shit"
	"cockburn":   true, // contains "cock" (and reads "Coburn")
	"penistone":  true, // contains "penis"
	"dickinson":  true, // contains "dick"
	"dickens":    true, // contains "dick"
}

// checkProfanity flags a callsign that contains a swear word — including across
// camelCase/word boundaries — or that sounds like one when spoken (catching
// phonetic spellings like "Phuck"). Any hit is CRITICAL: a profane callsign is
// disqualifying, not merely confusable. It returns at most one issue, naming the
// first matching swear word.
func checkProfanity(c string) []Issue {
	// Analyze the spoken form so digits read as words ("Sh1t" stays "Sh1t" for
	// the written substring check, but "Ass5" -> "AssFive" tokenizes cleanly).
	spoken := expandDigits(c)
	norm := normalize(spoken)
	if norm == "" || profanityAllowlist[norm] {
		return nil
	}

	// Spelled out: the swear word appears verbatim in the letters of the
	// callsign. normalize drops case and separators, so this catches it across
	// camelCase and punctuation boundaries ("GoldFucker", "Shit-Show").
	for _, w := range swearWords {
		if strings.Contains(norm, w) {
			return []Issue{{
				A: c, Severity: SevCritical, Kind: "profanity",
				Detail: fmt.Sprintf("contains the swear word %q", w),
			}}
		}
	}

	// Sounds like one: any spoken word token matches a swear word by ear, so
	// phonetic respellings ("Phuck", "Kunt") are caught too.
	for _, tok := range tokens(spoken) {
		if len(tok) < 3 {
			continue
		}
		for _, w := range swearWords {
			if soundsLikeWord(tok, w) {
				return []Issue{{
					A: c, Severity: SevCritical, Kind: "profanity",
					Detail: fmt.Sprintf("sounds like the swear word %q", w),
				}}
			}
		}
	}
	return nil
}

// soundsLikeMax is the phoneme distance at or below which a token counts as
// sounding like a listed word (swear word, proword): near-identical
// pronunciations only (e.g. Gold/Cold measure 0.02), tuned against the same
// battery as the pairwise thresholds once was — a looser bar would let the
// over-eager safety checks fire on merely similar words.
const soundsLikeMax = 0.06

// soundsLikeWord reports whether token tok sounds like the word w, using the
// same two-engine strategy as checkPair: the precise espeak-ng phoneme distance
// when available (so vowels distinguish "Sheet" from "Shit"), and the
// vowel-collapsing Metaphone 3 match as the fallback.
func soundsLikeWord(tok, w string) bool {
	if d, ok := phonetic.PhoneticDistance(tok, w); ok {
		return d <= soundsLikeMax
	}
	return phonetic.SoundsAlike(tok, w)
}
