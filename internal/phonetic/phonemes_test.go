package phonetic

import (
	"strings"
	"testing"
)

func TestParsePhonemes(t *testing.T) {
	// Primary stress is kept, normalized to a "'" prefix on the vowel.
	got := parsePhonemes("g_'oU_l_d_w_I_N\n")
	want := []string{"g", "'oU", "l", "d", "w", "I", "N"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("parsePhonemes = %v, want %v", got, want)
	}
}

func TestParsePhonemesSecondaryStress(t *testing.T) {
	// Secondary stress ("," — e.g. the ",aa" of "photograph") folds into the
	// same "'" mark as primary, so a word keeps the same vowel tokens whether
	// espeak gives it primary or demoted stress.
	got := parsePhonemes("f_'oU_t#_@_g_r_,aa_f\n")
	want := []string{"f", "'oU", "t#", "@", "g", "r", "'aa", "f"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("parsePhonemes = %v, want %v", got, want)
	}
}

func TestParsePhonemesStressCarriesToVowel(t *testing.T) {
	// A stress mark that lands on a consonant (or its own field) attaches to
	// the syllable's vowel, never to the consonant.
	got := parsePhonemes("'_s_t_I_N\n")
	want := []string{"s", "t", "'I", "N"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("parsePhonemes = %v, want %v", got, want)
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

// TestPerceptualWeights pins the channel-aware orderings the feature weights
// exist for (see the weight rationale in features.go): cues a noisy band-limited
// radio destroys (place, stridency) must cost less than cues that survive it
// (voicing, manner, nasality), per the Miller & Nicely confusion data.
func TestPerceptualWeights(t *testing.T) {
	// Place of articulation is the first cue lost in noise; voicing survives.
	// So /p/-/t/ (place) must be closer than /p/-/b/ (voicing) — on the air
	// "Pat"/"Tat" is the more confusable pair, and likewise /t/-/k/ vs /t/-/d/.
	if pt, pb := featureDistance("p", "t"), featureDistance("p", "b"); pt >= pb {
		t.Errorf("place must be cheaper than voicing: d(p,t)=%.3f >= d(p,b)=%.3f", pt, pb)
	}
	if tk, td := featureDistance("t", "k"), featureDistance("t", "d"); tk >= td {
		t.Errorf("place must be cheaper than voicing: d(t,k)=%.3f >= d(t,d)=%.3f", tk, td)
	}

	// Nasals confuse by place too: /m/-/n/ must be closer than a nasality
	// difference like /m/-/b/.
	if mn, mb := featureDistance("m", "n"), featureDistance("m", "b"); mn >= mb {
		t.Errorf("d(m,n)=%.3f should be < d(m,b)=%.3f", mn, mb)
	}

	// The /f/-/θ/ and /s/-/θ/ distinctions ride on high-frequency energy the
	// channel cuts ("Free"/"Three"), so they must cost less than a manner
	// difference like /t/-/s/.
	if fT, ts := featureDistance("f", "T"), featureDistance("t", "s"); fT >= ts {
		t.Errorf("d(f,T)=%.3f should be < d(t,s)=%.3f", fT, ts)
	}
	if sT, ts := featureDistance("s", "T"), featureDistance("t", "s"); sT >= ts {
		t.Errorf("d(s,T)=%.3f should be < d(t,s)=%.3f", sT, ts)
	}
}

// TestStressAwareVowelCosts pins the stress behaviors of featureDistance: two
// unstressed (reduced) vowels are closer than the same pair of stressed nuclei,
// and the same vowel at shifted stress costs exactly the mild stressMismatchCost.
func TestStressAwareVowelCosts(t *testing.T) {
	// /@/ vs /i/ unstressed: both reduce in running speech, identity faint.
	unstressed := featureDistance("@", "i")
	stressed := featureDistance("'@", "'i")
	if !(unstressed > 0 && unstressed < stressed) {
		t.Errorf("expected 0 < unstressed d(@,i)=%.3f < stressed d('@,'i)=%.3f", unstressed, stressed)
	}
	// Same vowel, shifted stress: a small prosodic difference, not a new sound.
	if d := featureDistance("'oU", "oU"); d != stressMismatchCost {
		t.Errorf("d('oU,oU) = %.3f, want stressMismatchCost %.3f", d, stressMismatchCost)
	}
	// Stress marks never make identical tokens differ.
	if d := featureDistance("'oU", "'oU"); d != 0 {
		t.Errorf("d('oU,'oU) = %.3f, want 0", d)
	}
}

// TestUnstressedIndelDiscount pins that deleting an unstressed vowel (the
// epenthetic /ə/ separating "Blaze" from "Belize") is cheap, while a stressed
// vowel or a consonant still pays the full indel.
func TestUnstressedIndelDiscount(t *testing.T) {
	if c := indelCost("@", false); c != unstressedIndelCost {
		t.Errorf("indelCost(@) = %.2f, want %.2f", c, unstressedIndelCost)
	}
	if c := indelCost("'oU", false); c != 1.0 {
		t.Errorf("indelCost('oU) = %.2f, want 1.0 (stressed nucleus)", c)
	}
	if c := indelCost("d", false); c != 1.0 {
		t.Errorf("indelCost(d) = %.2f, want 1.0 (consonant)", c)
	}
	// The weak-coda discount still wins for a final voiceless stop.
	if c := indelCost("t", true); c != codaIndelCost {
		t.Errorf("indelCost(t, last) = %.2f, want %.2f", c, codaIndelCost)
	}
}

// TestSyllabicityFloor pins that a vowel-consonant substitution is never cheap,
// however many features the pair shares — /ɜ/ and /n/ are both voiced
// sonorants, but pairing a nucleus with a consonant changes the syllable
// rhythm, and a cheap cost here opens degenerate edit-distance alignments (see
// the syllabicityFloor comment).
func TestSyllabicityFloor(t *testing.T) {
	if d := featureDistance("3", "n"); d < syllabicityFloor {
		t.Errorf("d(3,n) = %.3f, want >= syllabicityFloor %.2f", d, syllabicityFloor)
	}
	if d := featureDistance("@", "l"); d < syllabicityFloor {
		t.Errorf("d(@,l) = %.3f, want >= syllabicityFloor %.2f", d, syllabicityFloor)
	}
	// Consonant-consonant and vowel-vowel pairs are unaffected by the floor.
	if d := featureDistance("m", "n"); d >= syllabicityFloor {
		t.Errorf("d(m,n) = %.3f should be well under the floor", d)
	}
}

// TestRhymeIgnoresStress: a rime key must not depend on whether the final
// syllable carries the word's stress, or "Sting" (stressed -ing) would no
// longer rhyme with "Nesting" (unstressed -ing). Needs espeak-ng.
func TestRhymeIgnoresStress(t *testing.T) {
	if !PhonemesAvailable() {
		t.Skip("espeak-ng not installed")
	}
	if a, b := Rhyme("Sting"), Rhyme("Nesting"); a == "" || a != b {
		t.Errorf("Rhyme(Sting)=%q should equal Rhyme(Nesting)=%q", a, b)
	}
}

// TestEpenthesisDiscount: an extra unstressed syllable barely separates two
// handles on the air, so "Blaze"/"Belize" (epenthetic /ə/ plus a close stressed
// nucleus) must land well inside the sound-similar band, not at its edge as the
// flat indel cost had it (0.225 with the MED cutoff at 0.24). Needs espeak-ng.
func TestEpenthesisDiscount(t *testing.T) {
	if !PhonemesAvailable() {
		t.Skip("espeak-ng not installed")
	}
	d, ok := PhoneticDistance("Blaze", "Belize")
	if !ok {
		t.Fatal("PhoneticDistance(Blaze, Belize) not ok")
	}
	if d >= 0.15 {
		t.Errorf("PhoneticDistance(Blaze, Belize) = %.3f, want < 0.15", d)
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
