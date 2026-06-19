package phonetic

import (
	"strings"

	"github.com/dlclark/metaphone3"
)

// encode returns the primary and secondary Metaphone 3 keys for s.
//
// When encodeVowels is true, vowel positions are encoded (all vowels collapse
// to a single value, so vowel *position and count* are captured but not vowel
// identity). This makes the match stricter: two words must share both their
// consonants and their syllable structure. When false, only the consonant
// skeleton is compared, which is a looser signal.
//
// encodeExact is left at its default (false) so that voiced/unvoiced pairs
// (G/K, D/T, ...) collapse together — on the radio those genuinely sound
// confusable, which is exactly what we want to flag.
func encode(s string, encodeVowels bool) (primary, secondary string) {
	e := &metaphone3.Encoder{EncodeVowels: encodeVowels}
	return e.Encode(s)
}

// crossMatch reports whether any Metaphone 3 code of a (primary or secondary)
// equals any code of b. Cross-matching the alternate keys catches names with
// more than one valid pronunciation.
func crossMatch(a, b string, encodeVowels bool) bool {
	ap, as := encode(a, encodeVowels)
	bp, bs := encode(b, encodeVowels)
	for _, x := range [2]string{ap, as} {
		if x == "" {
			continue
		}
		for _, y := range [2]string{bp, bs} {
			if x == y {
				return true
			}
		}
	}
	return false
}

// SoundsAlike reports whether a and b sound nearly identical on the radio. It
// requires their Metaphone 3 keys to match with vowel positions included, so
// they share both consonants and syllable structure (e.g. "Knight" / "Nite",
// "Phipps" / "Fips", "Catherine" / "Katherine").
func SoundsAlike(a, b string) bool {
	return crossMatch(a, b, true)
}

// SoundsSimilar reports whether a and b share the same consonant skeleton
// (Metaphone 3 keys match without vowels) even though their vowel structure
// differs, e.g. "Blaze" / "Belize". This is a weaker signal than SoundsAlike;
// callers should treat a SoundsAlike pair as the stronger finding.
func SoundsSimilar(a, b string) bool {
	return crossMatch(a, b, false)
}

// SoundsLikeStartOf reports whether one word sounds like the beginning of the
// other (one consonant-skeleton key is a prefix of the other), e.g. "Gold" vs.
// "Goldwing". Weaker still than SoundsSimilar.
func SoundsLikeStartOf(a, b string) bool {
	ap, _ := encode(a, false)
	bp, _ := encode(b, false)
	if len(ap) < 2 || len(bp) < 2 || ap == bp {
		return false
	}
	return strings.HasPrefix(ap, bp) || strings.HasPrefix(bp, ap)
}
