package phonetic

import "strings"

// Rhyme returns a normalized "rime" for s: the final pronounced vowel and
// everything after it, used to detect rhyming callsigns ("Sting" / "GoldWing"
// both -> "ing"). A single silent trailing 'e' is dropped first so that "Nite"
// and "Kite" both rhyme on "it". Returns "" if there is no usable rime.
func Rhyme(s string) string {
	s = lettersLower(s)
	// Drop a single silent trailing 'e' after a consonant (magic-e words).
	if len(s) >= 3 && s[len(s)-1] == 'e' && !isVowelByte(s[len(s)-2]) {
		s = s[:len(s)-1]
	}
	lastGroup := -1
	for i := 0; i < len(s); i++ {
		if isVowelByte(s[i]) && (i == 0 || !isVowelByte(s[i-1])) {
			lastGroup = i
		}
	}
	if lastGroup < 0 {
		return ""
	}
	return s[lastGroup:]
}

// SyllableCount estimates the number of syllables in s using the standard
// vowel-group heuristic with a silent-'e' correction. It is approximate (no
// pronunciation dictionary), but consistent enough to compare two callsigns'
// cadence.
func SyllableCount(s string) int {
	s = lettersLower(s)
	if s == "" {
		return 0
	}
	count := 0
	prevVowel := false
	for i := 0; i < len(s); i++ {
		v := isVowelByte(s[i])
		// A 'y' between two vowels acts as a consonantal glide and splits
		// syllables rather than extending a vowel group (e.g. "Playa").
		if v && s[i] == 'y' && i > 0 && isVowelByte(s[i-1]) &&
			i+1 < len(s) && isVowelByte(s[i+1]) {
			v = false
		}
		if v && !prevVowel {
			count++
		}
		prevVowel = v
	}
	// Subtract a silent trailing 'e', but keep the syllable for a consonant +
	// "le" ending (e.g. "candle", "uncle").
	if strings.HasSuffix(s, "e") && count > 1 {
		if !(strings.HasSuffix(s, "le") && len(s) >= 3 && !isVowelByte(s[len(s)-3])) {
			count--
		}
	}
	if count == 0 {
		count = 1
	}
	return count
}
