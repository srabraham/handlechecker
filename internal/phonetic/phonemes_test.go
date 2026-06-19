package phonetic

import "testing"

func TestParsePhonemes(t *testing.T) {
	got := parsePhonemes("g_'oU_l_d_w_I_N\n")
	want := []string{"g", "oU", "l", "d", "w", "I", "N"}
	if len(got) != len(want) {
		t.Fatalf("parsePhonemes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parsePhonemes = %v, want %v", got, want)
		}
	}
}

func TestFeatureDistance(t *testing.T) {
	if featureDistance("b", "b") != 0 {
		t.Error("same phoneme should have zero distance")
	}
	// /b/ and /p/ differ only in voicing -> smaller than /b/ vs /k/.
	bp := featureDistance("b", "p")
	bk := featureDistance("b", "k")
	if !(bp > 0 && bp < bk) {
		t.Errorf("expected 0 < d(b,p)=%.3f < d(b,k)=%.3f", bp, bk)
	}
	if featureDistance("oU", "I") <= featureDistance("oU", "u:") {
		t.Error("oU should be closer to u: than to I")
	}
}

// TestPhonemeBattery only runs when espeak-ng is present; it prints distances so
// thresholds can be tuned, and asserts the basic ordering we rely on.
func TestPhonemeBattery(t *testing.T) {
	if !PhonemesAvailable() {
		t.Skip("espeak-ng not installed")
	}
	pairs := [][2]string{
		// should be near-identical
		{"Knight", "Nite"}, {"Gold", "Cold"}, {"Phipps", "Fips"},
		{"Catherine", "Katherine"}, {"Sun", "Son"},
		// vowel-quality difference (Metaphone 3 merges these; phonemes should not)
		{"Gold", "Gild"}, {"Ranger", "Ringer"}, {"Bat", "Bit"},
		// similar-ish
		{"Blaze", "Belize"}, {"Thunder", "Plunder"},
		// clearly different
		{"GoldWing", "Sunfire"}, {"Dust", "Playa"}, {"Thunder", "Lantern"},
	}
	for _, p := range pairs {
		d, ok := PhoneticDistance(p[0], p[1])
		if !ok {
			t.Errorf("PhoneticDistance(%q,%q) not ok", p[0], p[1])
			continue
		}
		pa, _ := phonemize(p[0])
		pb, _ := phonemize(p[1])
		t.Logf("%-22s d=%.3f   %v / %v", p[0]+"/"+p[1], d, pa, pb)
	}
}
