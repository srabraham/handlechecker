package checker

import (
	"strings"

	"github.com/srabraham/handlechecker/internal/phonetic"
)

// digitWords maps each decimal digit to the word an operator would speak for it
// on the air.
var digitWords = map[rune]string{
	'0': "Zero", '1': "One", '2': "Two", '3': "Three", '4': "Four",
	'5': "Five", '6': "Six", '7': "Seven", '8': "Eight", '9': "Nine",
}

// expandDigits rewrites each digit in a callsign as its spoken word, so that
// "Dog4" is analyzed the way it is read aloud: "DogFour". The substituted word
// is capitalized so that camelCase tokenization treats it as its own word
// ("Dog4" -> "DogFour" -> tokens "dog","four").
//
// This is applied before the sound- and spelling-based checks (NATO, phonetic,
// rhyme, edit distance, ...) so a digit and its word equivalent collide as they
// would on the radio. It is deliberately not applied to the written-roster
// checks (look-alike, confusable-chars), where the digit glyph itself is what
// matters.
func expandDigits(s string) string {
	if !strings.ContainsAny(s, "0123456789") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		if w, ok := digitWords[r]; ok {
			b.WriteString(w)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// PhonemesAvailable reports whether the espeak-ng phoneme engine (the preferred
// sound engine) is active. When false, the sound checks use the Metaphone 3
// fallback, which has no phonemes to show.
func PhonemesAvailable() bool { return phonetic.PhonemesAvailable() }

// PhonemeDebug describes how one callsign is sounded out for the phonetic
// checks: the spoken form actually phonemized (digits read as words) and its
// espeak-ng phoneme tokens. Phonemes is nil when espeak-ng is unavailable.
type PhonemeDebug struct {
	Callsign string
	Spoken   string
	Phonemes []string
}

// DebugPhonemes returns the phoneme breakdown for each callsign, exactly as the
// phonetic comparison sees it (digits expanded). It is intended for --debug
// output, not for analysis.
func DebugPhonemes(callsigns []string) []PhonemeDebug {
	out := make([]PhonemeDebug, 0, len(callsigns))
	for _, c := range callsigns {
		spoken := spokenForm(c)
		toks, _ := phonetic.Phonemes(spoken)
		out = append(out, PhonemeDebug{Callsign: c, Spoken: spoken, Phonemes: toks})
	}
	return out
}
