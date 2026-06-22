package phonetic

// SharedOpening measures how closely the opening consonants of a and b match —
// the onset cluster before each one's first vowel ("Hot" -> h, "Plunder" -> p l).
// It returns a normalized 0..1 distance and ok=false when espeak-ng is
// unavailable. A near-zero distance means the two callsigns begin with the same
// sound; combined with a shared rhyme this makes them confusable on the air even
// when their middles differ (e.g. "Hot Guy" / "HawkEye" — both open on /h/, both
// end on the "-y" rhyme), a pair whose whole-word distance otherwise looks safe
// because one carries an extra interior consonant.
//
// Only the onset *consonants* are compared, deliberately not the first vowel: the
// vowel feature distance is coarse enough to swamp the shared consonant (it rates
// "Hot"/"Hawk" no closer than "Mon"/"Bal"), whereas the onset consonant cleanly
// separates a genuine shared opening from a coincidental rhyme. A vowel-initial
// callsign has no onset to share, so a missing onset on either side scores 1.
func SharedOpening(a, b string) (dist float64, ok bool) {
	pa, err := phonemize(a)
	if err != nil || len(pa) == 0 {
		return 0, false
	}
	pb, err := phonemize(b)
	if err != nil || len(pb) == 0 {
		return 0, false
	}
	oa, ob := onset(pa), onset(pb)
	if len(oa) == 0 || len(ob) == 0 {
		return 1, true
	}
	n := len(oa)
	if len(ob) > n {
		n = len(ob)
	}
	return phonemeEditDistance(oa, ob) / float64(n), true
}

// onset returns the leading consonant phonemes before the first syllabic (vowel)
// phoneme. It is empty for a vowel-initial sequence (or one with no vowel).
func onset(toks []string) []string {
	for i, t := range toks {
		if f, ok := lookupFeatures(t); ok && f.syl {
			return toks[:i]
		}
	}
	return nil
}
