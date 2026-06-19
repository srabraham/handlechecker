package phonetic

import (
	"errors"
	"os/exec"
	"strings"
	"sync"
)

// This file adds optional phoneme-level sound comparison via espeak-ng. Unlike
// Metaphone (which collapses all vowels to one value), espeak-ng produces an
// actual pronunciation, so vowel quality is preserved. We then compute a
// feature-weighted edit distance over the phoneme sequences, where substituting
// two similar sounds (e.g. /b/ and /p/) costs less than two different ones.
//
// espeak-ng is an optional runtime dependency: if it is not installed, all of
// this degrades to a no-op and the caller falls back to Metaphone 3.

var errNoEspeak = errors.New("espeak-ng not available")

var (
	espeakOnce sync.Once
	espeakPath string
)

func espeakBin() string {
	espeakOnce.Do(func() {
		espeakPath, _ = exec.LookPath("espeak-ng")
	})
	return espeakPath
}

// PhonemesAvailable reports whether espeak-ng is installed and can be used for
// phoneme-level comparison.
func PhonemesAvailable() bool { return espeakBin() != "" }

// Phonemes returns the espeak-ng phoneme tokens for word (e.g. "goldwing" ->
// ["g","oU","l","d","w","I","N"]), the same sequence used by PhoneticDistance.
// ok is false when espeak-ng is unavailable or the word cannot be phonemized.
// Exposed for debug output.
func Phonemes(word string) (toks []string, ok bool) {
	t, err := phonemize(word)
	if err != nil || len(t) == 0 {
		return nil, false
	}
	return t, true
}

var (
	phonemeCacheMu sync.Mutex
	phonemeCache   = map[string][]string{}
)

// phonemize runs espeak-ng and returns the phoneme tokens for word, e.g.
// "goldwing" -> ["g","oU","l","d","w","I","N"]. Results are cached so that
// comparing every pair in a roster costs one espeak call per callsign, not per
// pair.
func phonemize(word string) ([]string, error) {
	bin := espeakBin()
	if bin == "" {
		return nil, errNoEspeak
	}
	phonemeCacheMu.Lock()
	defer phonemeCacheMu.Unlock()
	if toks, ok := phonemeCache[word]; ok {
		return toks, nil
	}
	// -q: no audio, -x: ASCII phonemes, --sep=_: delimit phonemes.
	cmd := exec.Command(bin, "-q", "-x", "--sep=_", "-v", "en-us")
	cmd.Stdin = strings.NewReader(word)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	toks := parsePhonemes(string(out))
	phonemeCache[word] = toks
	return toks, nil
}

// parsePhonemes splits espeak's "_"-separated output into bare phoneme tokens,
// dropping stress and boundary marks ('  ,  %  !  -).
func parsePhonemes(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == ' ' || r == '\n' || r == '\t' || r == '\r'
	})
	toks := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Map(func(r rune) rune {
			switch r {
			case '\'', '’', ',', '%', '!', '-', ';':
				return -1
			}
			return r
		}, f)
		// Drop a stray standalone length mark (":"); the real long vowels carry
		// it as part of a whole token like "i:" and are unaffected.
		if f != "" && f != ":" {
			toks = append(toks, f)
		}
	}
	return toks
}

// PhoneticDistance returns a normalized acoustic distance in [0,1] between the
// pronunciations of a and b (0 = identical sounding, 1 = maximally different).
// ok is false when espeak-ng is unavailable or either word cannot be
// phonemized.
func PhoneticDistance(a, b string) (dist float64, ok bool) {
	pa, err := phonemize(a)
	if err != nil || len(pa) == 0 {
		return 0, false
	}
	pb, err := phonemize(b)
	if err != nil || len(pb) == 0 {
		return 0, false
	}
	cost := phonemeEditDistance(pa, pb)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	return cost / float64(n), true
}

// phonemeEditDistance is a Levenshtein distance where substitution cost is the
// articulatory-feature distance between two phonemes (0..1) and insertion or
// deletion costs 1.
func phonemeEditDistance(a, b []string) float64 {
	prev := make([]float64, len(b)+1)
	cur := make([]float64, len(b)+1)
	for j := range prev {
		prev[j] = float64(j)
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = float64(i)
		for j := 1; j <= len(b); j++ {
			sub := prev[j-1] + featureDistance(a[i-1], b[j-1])
			del := prev[j] + 1
			ins := cur[j-1] + 1
			cur[j] = minf(sub, minf(del, ins))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// vowelWeight scales the cost of substituting one vowel for another. Vowels
// carry the syllable nucleus, so a vowel swap is perceptually larger than the
// raw feature count (diluted by word length) would suggest.
const vowelWeight = 2.0

// featureDistance returns the cost of substituting phoneme x with y: the
// fraction of articulatory features that differ (0 = same sound, 1 = unknown or
// totally different), scaled up for vowel-vowel substitutions.
func featureDistance(x, y string) float64 {
	if x == y {
		return 0
	}
	fx, okx := lookupFeatures(x)
	fy, oky := lookupFeatures(y)
	if !okx || !oky {
		return 1.0
	}
	d := float64(articDiff(fx, fy)) / numFeatures
	if fx.syl && fy.syl {
		d *= vowelWeight
	}
	return d
}

// lookupFeatures resolves a phoneme token to its feature vector, falling back to
// the base token when espeak appends a variant marker — a digit or a '#' (e.g.
// "I2" -> "I", "aI3" -> "aI", "I#" -> "I", "t#" -> "t").
func lookupFeatures(tok string) (artic, bool) {
	if f, ok := phonemeFeatures[tok]; ok {
		return f, true
	}
	if base := strings.TrimRight(tok, "0123456789#"); base != tok && base != "" {
		if f, ok := phonemeFeatures[base]; ok {
			return f, true
		}
	}
	return artic{}, false
}
