package phonetic

// artic is a compact articulatory feature vector for one phoneme. Each field is
// a binary distinctive feature; phonemes that share more features sound more
// alike. The values are approximate (especially for diphthongs) but consistent
// enough to drive a feature-weighted edit distance.
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

const numFeatures = 15.0

func articDiff(a, b artic) int {
	d := 0
	for _, p := range [...][2]bool{
		{a.syl, b.syl}, {a.son, b.son}, {a.voi, b.voi}, {a.nas, b.nas},
		{a.cont, b.cont}, {a.lab, b.lab}, {a.cor, b.cor}, {a.dor, b.dor},
		{a.strid, b.strid}, {a.lat, b.lat}, {a.high, b.high}, {a.low, b.low},
		{a.back, b.back}, {a.round, b.round}, {a.tense, b.tense},
	} {
		if p[0] != p[1] {
			d++
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
	// --- nasals ---
	"m": {son: true, voi: true, nas: true, lab: true},
	"n": {son: true, voi: true, nas: true, cor: true},
	"N": {son: true, voi: true, nas: true, dor: true},
	// --- approximants ---
	"l": {son: true, voi: true, cont: true, cor: true, lat: true},
	"r": {son: true, voi: true, cont: true, cor: true},
	"w": {son: true, voi: true, cont: true, lab: true, dor: true, high: true, back: true, round: true},
	"j": {son: true, voi: true, cont: true, dor: true, high: true},

	// --- monophthong vowels ---
	"i:": {syl: true, son: true, voi: true, cont: true, high: true, tense: true},
	"I":  {syl: true, son: true, voi: true, cont: true, high: true},
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
	"aI@": {syl: true, son: true, voi: true, cont: true, low: true, tense: true},
}
