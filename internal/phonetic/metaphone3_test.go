package phonetic

import "testing"

// TestEncodeVowelModes pins the two encoding modes down to concrete keys so a
// regression in metaphone3 (or our use of it) shows up here rather than as a
// silent shift in which pairs the checker flags.
func TestEncodeVowelModes(t *testing.T) {
	cases := []struct {
		in                  string
		wantVowel, wantSkel string // primary keys; secondary asserted separately
	}{
		{"Gold", "KALT", "KLT"},
		{"Goldwing", "KALTANK", "KLTNK"},
		{"Knight", "NAT", "NT"},
		{"Nite", "NAT", "NT"},
	}
	for _, c := range cases {
		if got, _ := encode(c.in, true); got != c.wantVowel {
			t.Errorf("encode(%q, vowels=true) primary = %q, want %q", c.in, got, c.wantVowel)
		}
		if got, _ := encode(c.in, false); got != c.wantSkel {
			t.Errorf("encode(%q, vowels=false) primary = %q, want %q", c.in, got, c.wantSkel)
		}
	}
}

// With vowels encoded, every vowel collapses to a single value ("A"), so the
// key captures vowel position/count but not vowel identity. That is the
// documented Metaphone limitation behind preferring the phoneme engine.
func TestEncodeCollapsesVowelIdentity(t *testing.T) {
	// Distinct vowels, same consonant skeleton and syllable shape -> same key.
	g, _ := encode("Gold", true)
	gi, _ := encode("Gild", true)
	if g != gi {
		t.Fatalf("encode collapses vowel identity: Gold=%q Gild=%q, want equal", g, gi)
	}
	if !SoundsAlike("Gold", "Gild") {
		t.Error("Gold/Gild collapse to the same key, so Metaphone reports them alike (known limitation)")
	}
	// "a" and "I" are different letters but both encode to the lone vowel value.
	a, _ := encode("a", true)
	i, _ := encode("I", true)
	if a != i {
		t.Errorf("single vowels should collapse: a=%q I=%q", a, i)
	}
}

// encodeExact is left false, so voiced/unvoiced consonant pairs collapse — on
// the radio Gold and Cold (G/K, D/T) sound confusable, which we want flagged.
func TestEncodeCollapsesVoicedUnvoiced(t *testing.T) {
	gold, _ := encode("Gold", true)
	cold, _ := encode("Cold", true)
	if gold != cold {
		t.Errorf("voiced/unvoiced should collapse: Gold=%q Cold=%q", gold, cold)
	}
	if !SoundsAlike("Gold", "Cold") {
		t.Error("Gold/Cold should sound alike (G/K and D/T collapse)")
	}
}

func TestEncodeEmptyAndNonLetters(t *testing.T) {
	for _, in := range []string{"", "123", "!!!"} {
		if p, s := encode(in, true); p != "" || s != "" {
			t.Errorf("encode(%q) = (%q,%q), want empty keys", in, p, s)
		}
	}
}

func TestCrossMatch(t *testing.T) {
	if !crossMatch("Knight", "Nite", true) {
		t.Error("Knight/Nite should cross-match with vowels")
	}
	if crossMatch("Gold", "Silver", true) {
		t.Error("Gold/Silver should not cross-match")
	}
	// Commutative: argument order must not matter.
	if crossMatch("Blaze", "Belize", false) != crossMatch("Belize", "Blaze", false) {
		t.Error("crossMatch should be commutative")
	}
}

// An empty key must never count as a match, even though both words "share" it.
func TestCrossMatchEmptyKeysNeverMatch(t *testing.T) {
	if crossMatch("", "", true) {
		t.Error("two empty inputs share an empty key but must not match")
	}
	if crossMatch("123", "456", true) {
		t.Error("non-letter inputs both encode empty; must not match")
	}
	if crossMatch("Gold", "", false) {
		t.Error("a word should not match the empty string")
	}
}

// The alternate-key cross-match is the whole reason crossMatch loops over both
// primary and secondary keys: these pairs match only because one word's
// *secondary* key equals the other's primary (their primaries differ).
func TestCrossMatchUsesSecondaryKey(t *testing.T) {
	pairs := [][2]string{
		{"Wagner", "Vagner"}, // Wagner secondary FAKNAR == Vagner primary
		{"Smith", "Schmidt"}, // Smith secondary XMAT == Schmidt primary
	}
	for _, p := range pairs {
		ap, _ := encode(p[0], true)
		bp, _ := encode(p[1], true)
		if ap == bp {
			t.Fatalf("%q/%q primaries already equal (%q); not exercising secondary path", p[0], p[1], ap)
		}
		if !crossMatch(p[0], p[1], true) {
			t.Errorf("crossMatch(%q,%q) = false, want true via secondary key", p[0], p[1])
		}
	}
}

func TestSoundsLikeStartOfGuards(t *testing.T) {
	// Equal skeletons are not a "start of" relationship.
	if SoundsLikeStartOf("Knight", "Nite") {
		t.Error("Knight/Nite have equal keys; should not report start-of")
	}
	if SoundsLikeStartOf("Gold", "Gold") {
		t.Error("a word is not the start of itself")
	}
	// Keys shorter than two characters are rejected (avoids spurious
	// single-consonant prefixes).
	if SoundsLikeStartOf("a", "Apple") {
		t.Error("one-character key should be rejected by the len<2 guard")
	}
	// Same first letter but diverging skeletons must not match.
	if SoundsLikeStartOf("Ed", "Echo") {
		t.Error("Ed/Echo share a letter but neither skeleton prefixes the other")
	}
	// Genuine prefix relationship, both directions.
	if !SoundsLikeStartOf("Gold", "Goldwing") {
		t.Error("Gold should sound like the start of Goldwing")
	}
	if SoundsLikeStartOf("Goldwing", "Gold") != SoundsLikeStartOf("Gold", "Goldwing") {
		t.Error("SoundsLikeStartOf should be commutative")
	}
}
