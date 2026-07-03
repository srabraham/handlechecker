package phonetic

// artic is a compact articulatory feature vector for one phoneme. Each field is
// a binary distinctive feature; two phonemes sound more alike the smaller the
// perceptually-weighted sum of the features on which they differ (see the
// weights below). The values are approximate (especially for diphthongs) but
// consistent enough to drive a feature-weighted edit distance.
type artic struct {
	syl   bool // syllabic (vowels)
	son   bool // sonorant (vowels, nasals, liquids, glides)
	voi   bool // voiced
	nas   bool // nasal
	cont  bool // continuant (fricatives, approximants, vowels)
	lab   bool // labial place
	cor   bool // coronal place
	dor   bool // dorsal place
	strid bool // strident
	lat   bool // lateral
	high  bool // high (vowels / palatal-ish consonants)
	low   bool // low/open (vowels)
	back  bool // back (vowels)
	round bool // rounded (vowels)
	tense bool // tense/long (vowels)
}

// Per-feature perceptual weights. A weight is the cost of two phonemes
// differing in that feature, and it models confusability over a *band-limited,
// noisy radio channel* (~300–3000 Hz), not articulatory bookkeeping: a cue that
// survives channel noise keeps two sounds apart on the air (high weight), while
// a cue the channel destroys does not (low weight), no matter how distinct the
// sounds are face to face. The ranking follows the classic consonant-confusion
// data (Miller & Nicely 1955, measured over exactly such a channel):
//
//   - Nasality and manner (sonorant, continuant) are recovered correctly even
//     at very low SNR — highest weights.
//   - Voicing is nearly as robust — high weight.
//   - Place of articulation (labial/coronal/dorsal) is the *first* cue lost in
//     noise: /p t k/, /b d g/, /m n/ confusions dominate the matrices — lowest
//     weight.
//   - Stridency mostly rides on high-frequency energy the channel cuts off
//     (the /f/–/θ/, /s/–/θ/ distinctions) — lowest weight.
//   - The vowel-geometry features (high/low/back/round/tense) stay at 1.0:
//     vowels are carried by formants the channel passes, and their relative
//     geometry is a separate concern (see the graded-vowel-distances roadmap
//     item). Vowel salience overall is handled by vowelWeight in phonemes.go.
//
// The weights sum to totalFeatureWeight, which featureDistance divides by, so
// the distance stays normalized to [0,1] and the overall scale (and thus the
// tuned thresholds in checker.go) is roughly preserved from the old uniform
// 1/15-per-feature scheme.
const (
	wSyl   = 2.0  // vowel vs consonant: structural, never misheard as each other
	wSon   = 1.5  // sonorant vs obstruent: manner, robust in noise
	wVoi   = 1.25 // voicing: robust in noise (M&N)
	wNas   = 1.75 // nasality: the most robust cue in the M&N matrices
	wCont  = 1.5  // stop vs continuant: manner, robust in noise
	wPlace = 0.5  // labial/coronal/dorsal: the first cue lost in noise
	wStrid = 0.5  // sibilance: high-frequency energy the channel cuts
	wLat   = 0.75 // l vs r: moderately confusable on the air
	wVowel = 1.0  // high/low/back/round/tense: vowel geometry, unchanged
)

const totalFeatureWeight = wSyl + wSon + wVoi + wNas + wCont +
	3*wPlace + wStrid + wLat + 5*wVowel // = 15.75

// articDist is the weighted sum of the features on which a and b differ, in
// [0, totalFeatureWeight].
func articDist(a, b artic) float64 {
	d := 0.0
	for _, p := range [...]struct {
		a, b bool
		w    float64
	}{
		{a.syl, b.syl, wSyl}, {a.son, b.son, wSon}, {a.voi, b.voi, wVoi},
		{a.nas, b.nas, wNas}, {a.cont, b.cont, wCont},
		{a.lab, b.lab, wPlace}, {a.cor, b.cor, wPlace}, {a.dor, b.dor, wPlace},
		{a.strid, b.strid, wStrid}, {a.lat, b.lat, wLat},
		{a.high, b.high, wVowel}, {a.low, b.low, wVowel}, {a.back, b.back, wVowel},
		{a.round, b.round, wVowel}, {a.tense, b.tense, wVowel},
	} {
		if p.a != p.b {
			d += p.w
		}
	}
	return d
}

// phonemeFeatures maps espeak-ng en-us phoneme tokens (as emitted by `-x
// --sep=_`, with stress marks stripped) to articulatory feature vectors.
var phonemeFeatures = map[string]artic{
	// --- stops ---
	"p": {lab: true},
	"b": {lab: true, voi: true},
	"t": {cor: true},
	"d": {cor: true, voi: true},
	"k": {dor: true},
	"g": {dor: true, voi: true},
	// --- affricates ---
	"tS": {cor: true, strid: true, high: true},
	"dZ": {cor: true, strid: true, high: true, voi: true},
	// --- glottal stop (e.g. "uh-oh") ---
	"?": {},
	// --- fricatives ---
	"f": {cont: true, lab: true, strid: true},
	"v": {cont: true, lab: true, strid: true, voi: true},
	"T": {cont: true, cor: true},
	"D": {cont: true, cor: true, voi: true},
	"s": {cont: true, cor: true, strid: true},
	"z": {cont: true, cor: true, strid: true, voi: true},
	"S": {cont: true, cor: true, strid: true, high: true},
	"Z": {cont: true, cor: true, strid: true, high: true, voi: true},
	"h": {cont: true},
	"C": {cont: true, dor: true, high: true}, // voiceless palatal fricative ("huge")
	"x": {cont: true, dor: true},             // voiceless velar fricative ("loch", "Bach")
	// --- nasals ---
	"m": {son: true, voi: true, nas: true, lab: true},
	"n": {son: true, voi: true, nas: true, cor: true},
	"N": {son: true, voi: true, nas: true, dor: true},
	// --- approximants ---
	"l":  {son: true, voi: true, cont: true, cor: true, lat: true},
	"@L": {syl: true, son: true, voi: true, cont: true, cor: true, lat: true}, // syllabic l ("bottle")
	"r":  {son: true, voi: true, cont: true, cor: true},
	"w":  {son: true, voi: true, cont: true, lab: true, dor: true, high: true, back: true, round: true},
	"j":  {son: true, voi: true, cont: true, dor: true, high: true},

	// --- monophthong vowels ---
	"i:": {syl: true, son: true, voi: true, cont: true, high: true, tense: true},
	"i":  {syl: true, son: true, voi: true, cont: true, high: true, tense: true}, // unstressed "happy" vowel
	"I":  {syl: true, son: true, voi: true, cont: true, high: true},
	"O":  {syl: true, son: true, voi: true, cont: true, low: true, back: true, round: true}, // open-o (ɔ)
	"e":  {syl: true, son: true, voi: true, cont: true},
	"E":  {syl: true, son: true, voi: true, cont: true},
	"a":  {syl: true, son: true, voi: true, cont: true, low: true},
	"a#": {syl: true, son: true, voi: true, cont: true, low: true},
	"{":  {syl: true, son: true, voi: true, cont: true, low: true},
	"A:": {syl: true, son: true, voi: true, cont: true, low: true, back: true, tense: true},
	"aa": {syl: true, son: true, voi: true, cont: true, low: true, back: true, tense: true},
	"V":  {syl: true, son: true, voi: true, cont: true, back: true},
	"Q":  {syl: true, son: true, voi: true, cont: true, low: true, back: true, round: true},
	"O:": {syl: true, son: true, voi: true, cont: true, low: true, back: true, round: true, tense: true},
	"U":  {syl: true, son: true, voi: true, cont: true, high: true, back: true, round: true},
	"u:": {syl: true, son: true, voi: true, cont: true, high: true, back: true, round: true, tense: true},
	"u":  {syl: true, son: true, voi: true, cont: true, high: true, back: true, round: true}, // lax/unstressed u
	"3":  {syl: true, son: true, voi: true, cont: true},
	"3:": {syl: true, son: true, voi: true, cont: true, tense: true},
	"@":  {syl: true, son: true, voi: true, cont: true},
	"0":  {syl: true, son: true, voi: true, cont: true},

	// --- diphthongs (approximated by overall trajectory) ---
	"aI":  {syl: true, son: true, voi: true, cont: true, low: true, tense: true},
	"aU":  {syl: true, son: true, voi: true, cont: true, low: true, back: true, round: true, tense: true},
	"OI":  {syl: true, son: true, voi: true, cont: true, back: true, round: true, tense: true},
	"eI":  {syl: true, son: true, voi: true, cont: true, tense: true},
	"oU":  {syl: true, son: true, voi: true, cont: true, back: true, round: true, tense: true},
	"@U":  {syl: true, son: true, voi: true, cont: true, back: true, round: true, tense: true},
	"e@":  {syl: true, son: true, voi: true, cont: true},
	"I@":  {syl: true, son: true, voi: true, cont: true, high: true},
	"i@":  {syl: true, son: true, voi: true, cont: true, high: true},
	"i@3": {syl: true, son: true, voi: true, cont: true, high: true},
	"U@":  {syl: true, son: true, voi: true, cont: true, high: true, back: true, round: true},
	"A@":  {syl: true, son: true, voi: true, cont: true, low: true, back: true},
	"O@":  {syl: true, son: true, voi: true, cont: true, low: true, back: true, round: true},
	"o@":  {syl: true, son: true, voi: true, cont: true, back: true, round: true}, // NORTH/FORCE vowel ("door")

	// --- nasalized vowels (loanwords, e.g. "genre", "bon") ---
	"A~":  {syl: true, son: true, voi: true, cont: true, nas: true, low: true, back: true, tense: true},
	"O~":  {syl: true, son: true, voi: true, cont: true, nas: true, low: true, back: true, round: true, tense: true},
	"aI@": {syl: true, son: true, voi: true, cont: true, low: true, tense: true},
}
