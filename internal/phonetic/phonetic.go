// Package phonetic implements phonetic and prosodic comparisons used to detect
// callsigns that sound alike when spoken over the radio, even if they are
// spelled differently.
//
// The headline "sounds the same" engine is Double Metaphone (see
// double_metaphone.go). Soundex is kept as a coarser fallback signal, and
// Rhyme/SyllableCount add prosodic comparisons.
package phonetic

import "strings"

// lettersUpper strips everything but ASCII letters and upper-cases the rest.
func lettersUpper(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteByte(c - ('a' - 'A'))
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c)
		}
	}
	return b.String()
}

// lettersLower strips everything but ASCII letters and lower-cases the rest.
func lettersLower(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c + ('a' - 'A'))
		case c >= 'a' && c <= 'z':
			b.WriteByte(c)
		}
	}
	return b.String()
}

func isVowelByte(c byte) bool {
	switch c {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	}
	return false
}

// Soundex returns the classic 4-character Soundex code for s. Words that share
// a Soundex code are a coarse signal that they may sound similar. Soundex is
// intentionally lossy and biased toward the first letter, so it is used here
// only as a fallback when Double Metaphone does not already flag a pair.
func Soundex(s string) string {
	s = lettersUpper(s)
	if s == "" {
		return ""
	}

	code := func(c byte) byte {
		switch c {
		case 'B', 'F', 'P', 'V':
			return '1'
		case 'C', 'G', 'J', 'K', 'Q', 'S', 'X', 'Z':
			return '2'
		case 'D', 'T':
			return '3'
		case 'L':
			return '4'
		case 'M', 'N':
			return '5'
		case 'R':
			return '6'
		}
		return '0' // vowels, H, W, Y
	}

	out := []byte{s[0]}
	prev := code(s[0])
	for i := 1; i < len(s) && len(out) < 4; i++ {
		c := s[i]
		d := code(c)
		if d != '0' && d != prev {
			out = append(out, d)
		}
		// H and W are transparent: they do not reset the "previous code" used
		// for the adjacency rule, but vowels do.
		if c != 'H' && c != 'W' {
			prev = d
		}
	}
	for len(out) < 4 {
		out = append(out, '0')
	}
	return string(out)
}
