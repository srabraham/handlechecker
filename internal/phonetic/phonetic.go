// Package phonetic implements phonetic and prosodic comparisons used to detect
// callsigns that sound alike when spoken over the radio, even if they are
// spelled differently.
//
// The "sounds the same" engine is Metaphone 3 (see metaphone3.go), which is
// more accurate than Double Metaphone and can optionally encode vowel positions
// — useful for telling apart names that share consonants. Rhyme and
// SyllableCount (see prosody.go) add complementary prosodic signals.
package phonetic

// lettersLower strips everything but ASCII letters and lower-cases the rest.
func lettersLower(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			b = append(b, c+('a'-'A'))
		case c >= 'a' && c <= 'z':
			b = append(b, c)
		}
	}
	return string(b)
}

func isVowelByte(c byte) bool {
	switch c {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	}
	return false
}
