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

	// "Abraham" -> eI b r @ h a m ; "Zebra" -> z i: b r @. The clean interior run
	// "b r @" (plus a near-matching front vowel) is clipped at both ends of both
	// words — Abraham keeps its "-ham", Zebra keeps its "z" — so neither word is
	// wholly contained and it must NOT count, even though the middle aligns at a
	// tiny distance. Without the whole-word guard this scored a spurious ~0.03.
	abraham := []string{"eI", "b", "r", "@", "h", "a", "m"}
	zebra := []string{"z", "i:", "b", "r", "@"}
	syl, dist = phonemeOverlap(abraham, zebra)
	if syl >= 2 && dist <= 0.12 {
		t.Errorf("Abraham/Zebra share only an interior run and must not count, got syl=%d dist=%.3f", syl, dist)
	}

	// "Ranger" -> r eI n dZ @ ; "Stranger" -> s t r eI n dZ @. The whole word
	// "Ranger" is heard at the END of "Stranger" (only the leading "st" differs),
	// so the run captures a whole word at an edge and SHOULD count — these are
	// genuinely confusable on the air.
	ranger := []string{"r", "eI", "n", "dZ", "@"}
	stranger := []string{"s", "t", "r", "eI", "n", "dZ", "@"}
	syl, dist = phonemeOverlap(ranger, stranger)
	if syl < 2 || dist > 0.12 {
		t.Errorf("Ranger inside Stranger should count as a clean overlap, got syl=%d dist=%.3f", syl, dist)
	}

	// "Frankenstein" -> f r a N k @ n s t aI n ; "Random" -> r a n d @ m. The
	// whole of "Random" loosely matches the *interior* "r a N k @ n" of
	// Frankenstein (walled in by "f" before and "stein" after) at a deceptively
	// low distance. A whole word buried mid-word in the other is not confusable,
	// so this must NOT count even though one word (Random) is wholly aligned.
	frankenstein := []string{"f", "r", "a", "N", "k", "@", "n", "s", "t", "aI", "n"}
	random := []string{"r", "a", "n", "d", "@", "m"}
	syl, dist = phonemeOverlap(frankenstein, random)
	if syl >= 2 && dist <= 0.12 {
		t.Errorf("Random is buried mid-word in Frankenstein and must not count, got syl=%d dist=%.3f", syl, dist)
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

	// Abraham/Zebra share only the interior "-bra-": neither word is wholly
	// contained in the other, so this must not register as an overlap. (Whole-word
	// comparison already rates them far apart at ~0.44; this guards the local
	// alignment from flagging the clipped middle.)
	if s, d, _ := PhoneticOverlap("Abraham", "Zebra"); s >= 2 && d <= 0.12 {
		t.Errorf("Abraham/Zebra share only an interior run and should not overlap, got syl=%d dist=%.3f", s, d)
	}

	// Ranger is heard whole at the end of Stranger, so this should still register.
	if s, d, ok := PhoneticOverlap("Ranger", "Stranger"); !ok || s < 2 || d > 0.12 {
		t.Errorf("Ranger inside Stranger should overlap, got syl=%d dist=%.3f ok=%v", s, d, ok)
	}

	// Random matches only a buried interior fragment of Frankenstein, so it must
	// not register despite the whole word aligning at a low distance.
	if s, d, _ := PhoneticOverlap("Frankenstein", "Random"); s >= 2 && d <= 0.12 {
		t.Errorf("Frankenstein/Random is a buried interior match and should not overlap, got syl=%d dist=%.3f", s, d)
	}
}
