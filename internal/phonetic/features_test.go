package phonetic

import (
	"strings"
	"testing"
)

// espeakInventory is the set of phoneme tokens espeak-ng (-x --sep=_ -v en-us)
// emits across a large English word list (the system dictionary plus a battery
// of loanwords, diphthongs, and r-colored vowels). Every one must resolve to a
// feature vector via lookupFeatures, so the feature-weighted distance never
// silently falls back to the "totally different" cost of 1.0. The stray length
// mark ":" is excluded: it is dropped in parsePhonemes, not mapped here.
var espeakInventory = strings.Fields(`
	? @ @2 @L 0 3 3: a A: A@ a# A~ aa aI aI@ aI3 aU
	b C d D dZ e E e@ eI f g h i I i: i@ i@3 I# I2 j
	k l l# m n N O O: o@ O@ O~ O2 OI oU p r s S t T
	t# t2 tS u U u: U@ v V w x z Z
`)

func TestFeatureMapCoversInventory(t *testing.T) {
	for _, tok := range espeakInventory {
		if _, ok := lookupFeatures(tok); !ok {
			t.Errorf("phoneme %q from espeak-ng has no feature vector", tok)
		}
	}
}

func TestStrayLengthMarkDropped(t *testing.T) {
	// A standalone ":" is an espeak length artifact, not a phoneme; it must be
	// dropped, while ":" inside a whole vowel token is kept.
	got := parsePhonemes("d_:_u:_n")
	want := []string{"d", "u:", "n"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("parsePhonemes dropped/kept the wrong tokens: got %v, want %v", got, want)
	}
}
