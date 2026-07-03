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
// ["g","'oU","l","d","w","I","N"] — a "'" prefix marks the stressed vowel), the
// same sequence used by PhoneticDistance. ok is false when espeak-ng is
// unavailable or the word cannot be phonemized. Exposed for debug output.
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
// "goldwing" -> ["g","'oU","l","d","w","I","N"] (stress kept, see
// parsePhonemes). Results are cached so that comparing every pair in a roster
// costs one espeak call per callsign, not per pair.
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

// parsePhonemes splits espeak's "_"-separated output into phoneme tokens,
// dropping boundary marks (%  !  -  ;) but keeping stress: espeak prefixes the
// stressed syllable's vowel with ' (primary) or , (secondary), and both are
// normalized to a "'" prefix on that vowel token ("b_E_l_'i:_z" ->
// ["b","E","l","'i:","z"]). Secondary stress folds into primary because espeak
// demotes a word's stress when it is embedded in a longer handle ("CCS" carries
// ",i:" where "CCEssay" carries "'i:"); a two-level distinction would make the
// same vowel look different across such pairs. A mark that lands on a
// non-syllabic token (or its own field) is carried forward to the syllable's
// vowel, so stress is only ever attached to a syllabic token.
func parsePhonemes(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == ' ' || r == '\n' || r == '\t' || r == '\r'
	})
	toks := make([]string, 0, len(fields))
	pending := false
	for _, f := range fields {
		f = strings.Map(func(r rune) rune {
			switch r {
			case '\'', '’', ',':
				pending = true
				return -1
			case '%', '!', '-', ';':
				return -1
			}
			return r
		}, f)
		// Drop a stray standalone length mark (":"); the real long vowels carry
		// it as part of a whole token like "i:" and are unaffected.
		if f == "" || f == ":" {
			continue
		}
		if pending {
			if ft, ok := lookupFeatures(f); ok && ft.syl {
				f = "'" + f
				pending = false
			}
		}
		toks = append(toks, f)
	}
	return toks
}

// splitStress separates a phoneme token from its normalized stress mark:
// "'i:" -> ("i:", true), "i:" -> ("i:", false).
func splitStress(tok string) (base string, stressed bool) {
	if strings.HasPrefix(tok, "'") {
		return tok[1:], true
	}
	return tok, false
}

// stripStress returns the tokens with stress marks removed. Used where segment
// identity matters but prosody does not (e.g. rime keys).
func stripStress(toks []string) []string {
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i], _ = splitStress(t)
	}
	return out
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

// codaIndelCost is the insertion/deletion cost charged for a sequence-final
// voiceless stop (e.g. the /t/ in "Set") instead of the flat 1.0. A word-final
// voiceless stop is perceptually faint — easily unreleased or lost on a noisy
// radio channel — so two words that match except for such a trailing consonant
// (e.g. "NullSet" / "Tulsa") should read as closer than a flat indel implies.
const codaIndelCost = 0.4

// phonemeEditDistance is a Levenshtein distance where substitution cost is the
// articulatory-feature distance between two phonemes (0..1) and insertion or
// deletion costs 1 — except a sequence-final weak coda (voiceless stop) costs
// codaIndelCost, since dropping such a trailing sound barely changes how a word
// is heard on the air.
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
			del := prev[j] + indelCost(a[i-1], i == len(a))
			ins := cur[j-1] + indelCost(b[j-1], j == len(b))
			cur[j] = minf(sub, minf(del, ins))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// unstressedIndelCost is the insertion/deletion cost charged for an unstressed
// vowel instead of the flat 1.0. Unstressed vowels reduce toward schwa and
// carry little identity on the air, so a word that differs from another only by
// an unstressed syllable nucleus (the epenthetic /ə/ that separates "Blaze"
// from "Belize") sounds much closer than a full indel implies. Same root cause
// as codaIndelCost: the edit exists on paper but is perceptually faint.
const unstressedIndelCost = 0.5

// indelCost is the cost of leaving tok unpaired: the flat 1.0, discounted to
// codaIndelCost when tok is the final token of its sequence and a weak coda (a
// voiceless stop), or to unstressedIndelCost when tok is an unstressed vowel —
// both are edits the ear largely misses.
func indelCost(tok string, last bool) float64 {
	if last && isWeakCoda(tok) {
		return codaIndelCost
	}
	if base, stressed := splitStress(tok); !stressed {
		if f, ok := lookupFeatures(base); ok && f.syl {
			return unstressedIndelCost
		}
	}
	return 1.0
}

// isWeakCoda reports whether tok is a voiceless stop (p, t, k, and the
// affricate tS) — a sound whose word-final realization is perceptually faint.
func isWeakCoda(tok string) bool {
	f, ok := lookupFeatures(tok)
	if !ok {
		return false
	}
	return !f.syl && !f.son && !f.cont && !f.voi && !f.nas
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// vowelWeight scales the cost of substituting one vowel for another when a
// *stressed* nucleus is involved. A stressed vowel carries the syllable the ear
// anchors on, so swapping it is perceptually larger than the raw feature count
// (diluted by word length) would suggest. Two *unstressed* vowels are not
// scaled: both reduce toward schwa in running speech, so their dictionary
// identity is faint on the air and the raw feature distance already overstates
// the difference.
const vowelWeight = 2.0

// stressMismatchCost is the flat cost added when two vowels differ in stress
// (and the full cost when they are otherwise the same vowel). A stress shift
// changes the word's prosodic envelope — one of the cues that survives channel
// noise best — but only mildly: two handles differing solely in stress
// placement still sound nearly identical on the air.
const stressMismatchCost = 0.1

// syllabicityFloor is the minimum substitution cost between a vowel and a
// consonant. Pairing a syllable nucleus with a consonant changes the word's
// syllable structure — the rhythm cue that survives channel noise best — so it
// must never be cheap, no matter how many features the two happen to share
// (/ɜ/ and /n/ are both voiced sonorants, raw distance 0.37). Without the floor
// the edit distance finds degenerate alignments: with unstressed-vowel indels
// discounted, "insert the unstressed vowel, then substitute vowel-for-
// consonant" undercuts the honest "match the vowels, insert the consonant"
// path ("Thunder"/"Lantern" collapsed from 0.26 to 0.24 that way). At 0.5 the
// floor also makes swScore in alignment.go non-positive, so a shared sound run
// can never extend across a vowel↔consonant pairing.
const syllabicityFloor = 0.5

// featureDistance returns the cost of substituting phoneme x with y: the
// perceptually-weighted fraction of articulatory features that differ (0 = same
// sound, 1 = unknown or totally different; see the feature weights in
// features.go). Vowel-vowel substitutions are stress-aware: a swap involving a
// stressed nucleus is scaled up by vowelWeight, two unstressed (reduced) vowels
// are not, and a stress mismatch adds stressMismatchCost (see those constants).
func featureDistance(x, y string) float64 {
	bx, sx := splitStress(x)
	by, sy := splitStress(y)
	if bx == by {
		if sx == sy {
			return 0
		}
		return stressMismatchCost // same sound, shifted stress
	}
	fx, okx := lookupFeatures(bx)
	fy, oky := lookupFeatures(by)
	if !okx || !oky {
		return 1.0
	}
	d := articDist(fx, fy) / totalFeatureWeight
	if fx.syl && fy.syl {
		if sx || sy {
			d *= vowelWeight
		}
		if sx != sy {
			d += stressMismatchCost
		}
	} else if fx.syl != fy.syl && d < syllabicityFloor {
		d = syllabicityFloor
	}
	return d
}

// lookupFeatures resolves a phoneme token to its feature vector, ignoring a
// normalized stress prefix ("'i:" -> "i:") and falling back to the base token
// when espeak appends a variant marker — a digit or a '#' (e.g. "I2" -> "I",
// "aI3" -> "aI", "I#" -> "I", "t#" -> "t").
func lookupFeatures(tok string) (artic, bool) {
	tok, _ = splitStress(tok)
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
