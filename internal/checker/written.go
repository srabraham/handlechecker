package checker

import "strings"

// homoglyphDigit maps digits and symbols to the letter they are most easily
// confused with on a written, printed, or handwritten roster.
var homoglyphDigit = map[rune]rune{
	'0': 'o', '1': 'l', '2': 'z', '3': 'e', '4': 'a',
	'5': 's', '6': 'g', '7': 't', '8': 'b', '9': 'g',
	'$': 's', '|': 'l', '!': 'l', '@': 'a',
}

// homoglyphReplacer collapses multi-character look-alikes that read as a single
// letter (e.g. "rn" looks like "m", "vv" like "w").
var homoglyphReplacer = strings.NewReplacer("rn", "m", "vv", "w", "cl", "d")

// homoglyphFold maps a callsign to a canonical form in which characters that
// look alike on paper collapse together. Two distinct callsigns with the same
// fold look effectively identical when written down, even if they are spelled
// differently (e.g. "G0LD" / "GOLD", "Sl1ce" / "Slice", "Moder" + "rn").
func homoglyphFold(s string) string {
	s = homoglyphReplacer.Replace(strings.ToLower(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case homoglyphDigit[r] != 0:
			b.WriteRune(homoglyphDigit[r])
		}
	}
	return b.String()
}

// hasConfusableChars reports whether s contains characters that are easily
// misread as letters on a written roster (digits or look-alike symbols).
func hasConfusableChars(s string) bool {
	for _, r := range strings.ToLower(s) {
		if homoglyphDigit[r] != 0 {
			return true
		}
	}
	return false
}
