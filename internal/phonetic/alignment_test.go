package phonetic

import "testing"

// TestPhonemeOverlap exercises the local-alignment core directly on phoneme
// tokens, so it runs without espeak-ng installed.
func TestPhonemeOverlap(t *testing.T) {
	// "DustyDog" -> d ʌ s t i | d ɒ g ; "ADustyLog" -> ə | d ʌ s t i | l ɒ g.
	// They share the "Dusty" core (d ʌ s t i), a clean 2-syllable run, despite
	// the leading schwa and the Dog/Log tail.
	dustyDog := []string{"d", "V", "s", "t", "I", "d", "Q", "g"}
	aDustyLog := []string{"@", "d", "V", "s", "t", "I", "l", "Q", "g"}
	syl, dist := phonemeOverlap(dustyDog, aDustyLog)
	if syl < 2 {
		t.Errorf("expected a >=2-syllable shared run, got %d (dist %.3f)", syl, dist)
	}
	if dist > 0.12 {
		t.Errorf("expected the shared run to match cleanly, got dist %.3f", dist)
	}

	// Unrelated words should not share a clean multi-syllable run.
	thunder := []string{"T", "V", "n", "d", "3"}
	playa := []string{"p", "l", "aI", "@"}
	syl, dist = phonemeOverlap(thunder, playa)
	if syl >= 2 && dist <= 0.12 {
		t.Errorf("Thunder/Playa should not share a clean multi-syllable run, got syl=%d dist=%.3f", syl, dist)
	}
}

// TestPhoneticOverlapEspeak is the case we most care about: with espeak-ng
// present, a shared multi-syllable core is detected end-to-end even when the
// callsigns differ at the edges.
func TestPhoneticOverlapEspeak(t *testing.T) {
	if !PhonemesAvailable() {
		t.Skip("espeak-ng not installed")
	}
	syl, dist, ok := PhoneticOverlap("DustyDog", "ADustyLog")
	if !ok {
		t.Fatal("PhoneticOverlap not ok with espeak-ng available")
	}
	if syl < 2 || dist > 0.12 {
		t.Errorf("DustyDog/ADustyLog: expected a clean >=2-syllable overlap, got syl=%d dist=%.3f", syl, dist)
	}
	// A clearly different pair should not.
	if s, d, _ := PhoneticOverlap("DustyDog", "Sunfire"); s >= 2 && d <= 0.12 {
		t.Errorf("DustyDog/Sunfire should not overlap, got syl=%d dist=%.3f", s, d)
	}
}
