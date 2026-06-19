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
	cases := map[string]string{
		"GoldWing": "ing",
		"Sting":    "ing",
		"Nite":     "it",
		"Kite":     "it",
		"Thunder":  "er",
	}
	for in, want := range cases {
		if got := Rhyme(in); got != want {
			t.Errorf("Rhyme(%q) = %q, want %q", in, got, want)
		}
	}
	if Rhyme("GoldWing") != Rhyme("Sting") {
		t.Error("GoldWing and Sting should rhyme")
	}
	if Rhyme("Nite") != Rhyme("Kite") {
		t.Error("Nite and Kite should rhyme")
	}
}

func TestSyllableCount(t *testing.T) {
	cases := map[string]int{
		"Gold":      1,
		"GoldWing":  2,
		"Thunder":   2,
		"Catherine": 3,
		"Playa":     2,
		"Candle":    2, // silent-e exception for consonant + "le"
		"Nite":      1, // silent trailing e
	}
	for in, want := range cases {
		if got := SyllableCount(in); got != want {
			t.Errorf("SyllableCount(%q) = %d, want %d", in, got, want)
		}
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
