package phonetic

import "testing"

func TestSoundsAlike(t *testing.T) {
	// Spelled differently, sound the same.
	yes := [][2]string{
		{"Knight", "Nite"},
		{"Catherine", "Katherine"},
		{"Wright", "Rite"},
		{"Phipps", "Fips"},
		{"GoldWing", "goldwing"},
	}
	for _, p := range yes {
		if !SoundsAlike(p[0], p[1]) {
			t.Errorf("SoundsAlike(%q,%q) = false, want true", p[0], p[1])
		}
	}
	no := [][2]string{
		{"GoldWing", "Sunfire"},
		{"Dust", "Playa"},
		{"Thunder", "Lantern"},
	}
	for _, p := range no {
		if SoundsAlike(p[0], p[1]) {
			t.Errorf("SoundsAlike(%q,%q) = true, want false", p[0], p[1])
		}
	}
}

func TestSoundsSimilar(t *testing.T) {
	// Same consonant skeleton, different vowel structure: similar but not
	// near-identical.
	if !SoundsSimilar("Blaze", "Belize") {
		t.Error("Blaze/Belize should share a consonant skeleton")
	}
	if SoundsAlike("Blaze", "Belize") {
		t.Error("Blaze/Belize differ in vowel structure; should not be near-identical")
	}
	if SoundsSimilar("GoldWing", "Sunfire") {
		t.Error("GoldWing/Sunfire should not be similar")
	}
}

func TestSoundsLikeStartOf(t *testing.T) {
	if !SoundsLikeStartOf("Gold", "Goldwing") {
		t.Error("expected Gold to sound like the start of Goldwing")
	}
	if SoundsLikeStartOf("Gold", "Silver") {
		t.Error("Gold/Silver should not match as sound-prefix")
	}
}

func TestRhyme(t *testing.T) {
	// Rhyme keys are opaque and engine-dependent (espeak phonemes when present,
	// a spelling heuristic otherwise), so assert the relationships that matter
	// rather than exact keys — these hold under both engines.
	rhyming := [][2]string{{"GoldWing", "Sting"}, {"Nite", "Kite"}}
	for _, p := range rhyming {
		if Rhyme(p[0]) == "" || Rhyme(p[0]) != Rhyme(p[1]) {
			t.Errorf("expected %q and %q to rhyme, got %q / %q", p[0], p[1], Rhyme(p[0]), Rhyme(p[1]))
		}
	}
	if Rhyme("GoldWing") == Rhyme("Thunder") {
		t.Error("GoldWing and Thunder should not rhyme")
	}
}

func TestSyllableCount(t *testing.T) {
	// Counts that agree between the espeak phoneme reading and the spelling
	// heuristic, so the test holds whether or not espeak-ng is installed.
	cases := map[string]int{
		"Gold":     1,
		"GoldWing": 2,
		"Thunder":  2,
		"Playa":    2,
		"Candle":   2, // syllabic 'l' / silent-e consonant + "le"
		"Nite":     1, // silent trailing e
	}
	for in, want := range cases {
		if got := SyllableCount(in); got != want {
			t.Errorf("SyllableCount(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSyllableCountPhoneme(t *testing.T) {
	if !PhonemesAvailable() {
		t.Skip("espeak-ng not installed; phoneme syllable counting is not exercised")
	}
	// espeak reads "Catherine" as two syllables ("CATH-rin"), which the
	// vowel-group spelling heuristic (3) gets wrong — the phoneme path is more
	// faithful to how it is actually spoken.
	if got := SyllableCount("Catherine"); got != 2 {
		t.Errorf("phoneme SyllableCount(Catherine) = %d, want 2", got)
	}
}

func TestEmpty(t *testing.T) {
	if SoundsAlike("", "") || SoundsSimilar("", "") {
		t.Error("empty input should not sound alike")
	}
	if Rhyme("123") != "" || SyllableCount("") != 0 {
		t.Error("non-letter input should produce empty rhyme / zero syllables")
	}
}
