package phonetic

import "strings"

// Rhyme returns a normalized "rime" for s: the final pronounced vowel and
// everything after it, used to detect rhyming callsigns ("Sting" / "GoldWing"
// both -> "...IN"). When espeak-ng is available it works from the real phoneme
// sequence (so silent letters and digraphs are handled — "Through"/"Cough" no
// longer fool it); otherwise it falls back to a spelling heuristic. The returned
// key is opaque and engine-dependent: callers should only compare two rimes for
// equality, never parse them.
func Rhyme(s string) string {
	if r, ok := phonemeRhyme(s); ok {
		return r
	}
	return spellingRhyme(s)
}

// SyllableCount estimates the number of syllables in s. When espeak-ng is
// available it counts syllabic phonemes in the real pronunciation; otherwise it
// uses the vowel-group spelling heuristic. Both are approximate but consistent
// enough to compare two callsigns' cadence and to gate the 2–5 syllable range.
func SyllableCount(s string) int {
	if n, ok := phonemeSyllableCount(s); ok {
		return n
	}
	return spellingSyllableCount(s)
}

// --- phoneme-based implementations (espeak-ng) -------------------------------

// triphthongs are wide diphthong-plus-schwa vowels that espeak emits as a single
// phoneme token ("fire", "playa" -> "aI@") but which carry two syllables. They
// get an extra syllable so such words are not undercounted.
var triphthongs = map[string]bool{"aI@": true, "aU@": true, "OI@": true}

// isSyllabic reports whether a phoneme token is a syllable nucleus (a vowel or a
// syllabic consonant like "@L").
func isSyllabic(tok string) bool {
	f, ok := lookupFeatures(tok)
	return ok && f.syl
}

// phonemeSyllableCount counts syllable nuclei in the espeak pronunciation of s.
// ok is false — so the caller falls back to the spelling heuristic rather than
// trust a partial reading — when espeak is unavailable, s has no letters, or any
// phoneme token is unrecognized.
func phonemeSyllableCount(s string) (int, bool) {
	if lettersLower(s) == "" {
		return 0, false
	}
	toks, ok := Phonemes(s)
	if !ok {
		return 0, false
	}
	n := 0
	for _, t := range stripStress(toks) {
		if _, known := lookupFeatures(t); !known {
			return 0, false
		}
		if isSyllabic(t) {
			n++
		}
		if triphthongs[t] {
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return n, true
}

// phonemeRhyme returns the rime — the final syllabic phoneme and everything after
// it — from the espeak pronunciation of s, joined into a comparison key. Stress
// marks are stripped from the key: whether the final syllable happens to carry
// the word's stress ("Sting" vs "GoldWing") doesn't change what it rhymes with.
// ok is false when espeak is unavailable, s has no letters, or there is no vowel.
func phonemeRhyme(s string) (string, bool) {
	if lettersLower(s) == "" {
		return "", false
	}
	toks, ok := Phonemes(s)
	if !ok {
		return "", false
	}
	last := -1
	for i, t := range toks {
		if isSyllabic(t) {
			last = i
		}
	}
	if last < 0 {
		return "", false
	}
	return strings.Join(stripStress(toks[last:]), ""), true
}

// --- spelling heuristics (fallback when espeak-ng is absent) ------------------

// spellingRhyme is the letters-only rime: the final vowel group and everything
// after it. A single silent trailing 'e' after a consonant is dropped first so
// "Nite"/"Kite" both rhyme on "it". Returns "" if there is no usable rime.
func spellingRhyme(s string) string {
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

// spellingSyllableCount is the vowel-group heuristic with a silent-'e'
// correction.
func spellingSyllableCount(s string) int {
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
