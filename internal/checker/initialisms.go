package checker

import "strings"

// letterNames maps each letter to the way it is spoken aloud on its own — the
// "spell it out" pronunciation an operator uses for an initialism ("FBI" ->
// "ef bee eye"). Each name is Title-cased and free of internal capitals so that
// the camelCase tokenizer treats one spelled letter as one word token.
var letterNames = map[rune]string{
	'A': "Ay", 'B': "Bee", 'C': "See", 'D': "Dee", 'E': "Ee", 'F': "Ef",
	'G': "Gee", 'H': "Aitch", 'I': "Eye", 'J': "Jay", 'K': "Kay", 'L': "El",
	'M': "Em", 'N': "En", 'O': "Oh", 'P': "Pee", 'Q': "Cue", 'R': "Ar",
	'S': "Ess", 'T': "Tee", 'U': "You", 'V': "Vee", 'W': "Doubleyou",
	'X': "Ex", 'Y': "Why", 'Z': "Zee",
}

// expandInitialisms rewrites all-caps letter runs as their spoken letter names,
// so that a callsign meant to be spelled out is analyzed the way it is read
// aloud: "S A" -> "Ess Ay", "USB Key" -> "You Ess Bee Key". This is what keeps
// "S A" from being treated as the syllable "sa" (and matching the "-sa" in
// "Tulsa"); on the air it is "ess ay".
//
// A maximal run of uppercase letters is spelled out UNLESS it is immediately
// followed by a lowercase letter — in that case the run is the onset of an
// ordinary word (the "G" of "Gold", the "GB" of "GBush", the "USBK" of
// "USBKey") and is left untouched. So only unambiguous initialisms are
// expanded: fully-uppercase tokens ("USB", "LL"), separator-delimited single
// letters ("S A"), and trailing/standalone capitals ("GoldX" -> "GoldEx").
// Glued mixed-case forms like "GBush" or "USBKey" are deliberately not guessed
// at — see CLAUDE.md and TODO.md for the input/spacing question they raise.
//
// It is applied before expandDigits so that digit words (which are Title-cased,
// e.g. "One") cannot glue onto and mask an adjacent acronym run, and so that a
// lone letter beside a digit reads correctly: "R2D2" -> "Ar2Dee2" ->
// "ArTwoDeeTwo". Like expandDigits it feeds the sound- and spelling-based
// checks, not the written-roster checks (where the literal glyphs matter).
func expandInitialisms(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r >= 'A' && r <= 'Z' }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	rs := []rune(s)
	for i := 0; i < len(rs); {
		if rs[i] < 'A' || rs[i] > 'Z' {
			b.WriteRune(rs[i])
			i++
			continue
		}
		// Maximal run of uppercase ASCII letters [i, j).
		j := i
		for j < len(rs) && rs[j] >= 'A' && rs[j] <= 'Z' {
			j++
		}
		// A run immediately followed by a lowercase letter is the start of an
		// ordinary word, not an initialism — leave it verbatim.
		if j < len(rs) && rs[j] >= 'a' && rs[j] <= 'z' {
			b.WriteString(string(rs[i:j]))
		} else {
			for _, r := range rs[i:j] {
				b.WriteString(letterNames[r])
			}
		}
		i = j
	}
	return b.String()
}

// spokenForm renders a callsign the way it is read aloud for the sound- and
// spelling-based checks: initialisms spelled out, then digits read as words.
// "K9" -> "KayNine", "S A" -> "EssAy", "Dog4" -> "DogFour".
func spokenForm(s string) string {
	return expandDigits(expandInitialisms(s))
}
