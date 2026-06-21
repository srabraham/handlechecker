package phonetic

// This file adds a local (Smith-Waterman) alignment over phoneme sequences. The
// global PhoneticDistance normalizes by the longer word, so a strong shared
// core gets diluted when the two callsigns differ at the edges: "DustyDog" and
// "ADustyLog" both say "Dusty", but the leading "A" and the Dog/Log tail make
// the global distance look safe. Local alignment instead finds the best-matching
// contiguous run of sounds, ignoring unmatched prefixes and suffixes, so a
// shared multi-syllable run still raises a flag.

const (
	// swMatch is the score of a perfectly matching phoneme pair; the reward
	// shrinks with feature distance and goes negative for poor matches so the
	// aligned run does not extend through dissimilar sounds.
	swMatch = 1.0
	// swGap penalizes an insertion or deletion within the aligned run.
	swGap = 0.7
)

// swScore is the substitution score for aligning phoneme x with y: +swMatch when
// identical, 0 at feature distance 0.5, negative beyond that.
func swScore(x, y string) float64 {
	return swMatch * (1 - 2*featureDistance(x, y))
}

// PhoneticOverlap finds the best-matching contiguous run of sounds shared by a
// and b via local alignment of their phoneme sequences. It returns the number
// of syllables (vowels) that run spans and a normalized 0..1 distance measuring
// how cleanly it matches. Two callsigns that share a multi-syllable core (e.g.
// "DustyDog" / "ADustyLog") are easily confused on the air even when their
// overall lengths differ enough that PhoneticDistance looks safe. ok is false
// when espeak-ng is unavailable.
//
// To count, the run must capture one word whole at the start or end of the other
// (see phonemeOverlap); an interior-only match, or a whole word buried mid-word
// in the other, is reported as no overlap (syllables 0, distance 1).
func PhoneticOverlap(a, b string) (syllables int, dist float64, ok bool) {
	pa, err := phonemize(a)
	if err != nil || len(pa) == 0 {
		return 0, 0, false
	}
	pb, err := phonemize(b)
	if err != nil || len(pb) == 0 {
		return 0, 0, false
	}
	s, d := phonemeOverlap(pa, pb)
	return s, d, true
}

// phonemeOverlap locates the best local alignment between two phoneme sequences
// and reports its shared-syllable count and normalized feature distance. Smith-
// Waterman only locates the run; the run itself is scored with the same
// feature-weighted edit distance used by PhoneticDistance, so the reported
// number is comparable to the global one.
func phonemeOverlap(pa, pb []string) (syllables int, dist float64) {
	i0, i1, j0, j1 := localAlign(pa, pb)
	if i1 <= i0 {
		return 0, 1
	}
	// The overlap only counts when one callsign is heard, complete, at the START
	// or END of the other — e.g. "DustyDog" at the tail of "ADustyLog", or
	// "Ranger" at the tail of "Stranger". Two guards enforce that:
	//
	//   1. Whole word: the run must span an entire word on one side. A run clipped
	//      at both ends of *both* words drops each word's distinguishing onset
	//      and/or coda — the cues that keep them apart on the air. "Abraham" and
	//      "Zebra" share only the interior "-bra-" (Abraham loses its "-ham", Zebra
	//      its "Z"), so neither is whole; without this they scored a spurious 0.03.
	//
	//   2. At an edge: that whole word must align to a prefix or suffix of the
	//      other, not a buried interior fragment. The whole of "Random"
	//      (r a n d @ m) loosely matches the interior "...ranken..." of
	//      "Frankenstein" (f|r a N k @ n|s t aI n) at a low 0.08, but it is walled
	//      in by "f" before and "stein" after, so it is not confusable.
	wholeA := i0 == 0 && i1 == len(pa)
	wholeB := j0 == 0 && j1 == len(pb)
	edgeA := i0 == 0 || i1 == len(pa) // run touches a prefix/suffix of a
	edgeB := j0 == 0 || j1 == len(pb) // run touches a prefix/suffix of b
	if !((wholeA && edgeB) || (wholeB && edgeA)) {
		return 0, 1
	}
	subA, subB := pa[i0:i1], pb[j0:j1]
	n := len(subA)
	if len(subB) > n {
		n = len(subB)
	}
	syl := countSyllables(subA)
	if s := countSyllables(subB); s < syl {
		syl = s
	}
	return syl, phonemeEditDistance(subA, subB) / float64(n)
}

// localAlign runs Smith-Waterman over the two phoneme sequences and returns the
// half-open ranges [ai0:ai1] and [bj0:bj1] of the best-scoring local alignment.
// All-zero ranges mean no run scored above zero.
func localAlign(a, b []string) (ai0, ai1, bj0, bj1 int) {
	n, m := len(a), len(b)
	h := make([][]float64, n+1)
	dir := make([][]byte, n+1) // 0=stop, 1=diag, 2=up (consume a), 3=left (consume b)
	for i := range h {
		h[i] = make([]float64, m+1)
		dir[i] = make([]byte, m+1)
	}
	best := 0.0
	var bi, bj int
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			diag := h[i-1][j-1] + swScore(a[i-1], b[j-1])
			up := h[i-1][j] - swGap
			left := h[i][j-1] - swGap
			v, d := 0.0, byte(0)
			if diag > v {
				v, d = diag, 1
			}
			if up > v {
				v, d = up, 2
			}
			if left > v {
				v, d = left, 3
			}
			h[i][j], dir[i][j] = v, d
			if v > best {
				best, bi, bj = v, i, j
			}
		}
	}
	if best == 0 {
		return 0, 0, 0, 0
	}
	// Trace back from the highest-scoring cell to where the run began (the first
	// cell that contributed nothing). The boundary cell (i,j) is excluded, so the
	// run is a[i:bi] / b[j:bj].
	i, j := bi, bj
	for i > 0 && j > 0 && dir[i][j] != 0 {
		switch dir[i][j] {
		case 1:
			i, j = i-1, j-1
		case 2:
			i--
		case 3:
			j--
		}
	}
	return i, bi, j, bj
}

// countSyllables counts the syllabic (vowel) phonemes in a token sequence.
func countSyllables(toks []string) int {
	n := 0
	for _, t := range toks {
		if f, ok := lookupFeatures(t); ok && f.syl {
			n++
		}
	}
	return n
}
