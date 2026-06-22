package checker

import (
	"strings"
	"testing"

	"github.com/srabraham/handlechecker/internal/phonetic"
)

// soundIssues returns the sound-related findings (kind prefixed "sound").
func soundIssues(issues []Issue) []Issue {
	var out []Issue
	for _, is := range issues {
		if strings.HasPrefix(is.Kind, "sound") {
			out = append(out, is)
		}
	}
	return out
}

// hasKind reports whether any issue involving the given subjects has the kind.
func hasKind(issues []Issue, kind string) bool {
	for _, is := range issues {
		if is.Kind == kind {
			return true
		}
	}
	return false
}

func TestSharedPrefixWord(t *testing.T) {
	issues := checkPair("GoldWing", "GoldBar")
	if !hasKind(issues, "shared-word") {
		t.Errorf("expected shared-word for GoldWing/GoldBar, got %+v", issues)
	}
}

func TestDuplicate(t *testing.T) {
	issues := checkPair("Gold-Wing", "goldwing")
	if len(issues) != 1 || issues[0].Kind != "duplicate" {
		t.Errorf("expected single duplicate issue, got %+v", issues)
	}
}

func TestEditDistance(t *testing.T) {
	if !hasKind(checkPair("Spark", "Sparc"), "edit-distance") {
		t.Error("expected edit-distance issue for Spark/Sparc")
	}
}

func TestSoundAlike(t *testing.T) {
	// Different spelling, same sound.
	if !hasKind(checkPair("Knight", "Nite"), "sound-alike") {
		t.Error("expected sound-alike for Knight/Nite")
	}
}

// TestMetaphoneEscalation checks the both-engines behavior: when espeak-ng is
// available, the phoneme engine and Metaphone 3 both run. "Gild" and "Gold"
// differ only in their vowel, so the vowel-aware phoneme engine rates them a
// MEDIUM sound-similar, while vowel-collapsing Metaphone rates them a HIGH
// sound-alike. We surface both — the precise distance and Metaphone's stronger
// warning — rather than letting the phoneme verdict hide the conflict.
func TestMetaphoneEscalation(t *testing.T) {
	if !phonetic.PhonemesAvailable() {
		t.Skip("espeak-ng not installed; the both-engines escalation path is not exercised")
	}
	sounds := soundIssues(checkPair("Gild", "Gold"))
	var high, med bool
	for _, is := range sounds {
		if is.Kind == "sound-alike" && is.Severity == SevHigh {
			high = true
		}
		if is.Kind == "sound-similar" && is.Severity == SevMedium {
			med = true
		}
	}
	if !high {
		t.Errorf("expected a HIGH sound-alike (Metaphone escalation) for Gild/Gold, got %+v", sounds)
	}
	if !med {
		t.Errorf("expected a MEDIUM sound-similar (phoneme distance) for Gild/Gold, got %+v", sounds)
	}
}

// TestNoDuplicateSound checks that when the phoneme engine already rates a pair
// as a HIGH sound-alike, Metaphone does not pile on a second, redundant sound
// finding. "Cold" and "Gold" are phonetically near-identical (distance ~0.02),
// so only the single phoneme finding should survive.
func TestNoDuplicateSound(t *testing.T) {
	if !phonetic.PhonemesAvailable() {
		t.Skip("espeak-ng not installed; the de-duplication path needs the phoneme engine")
	}
	sounds := soundIssues(checkPair("Cold", "Gold"))
	if len(sounds) != 1 {
		t.Fatalf("expected exactly one sound finding for Cold/Gold, got %d: %+v", len(sounds), sounds)
	}
	if !strings.Contains(sounds[0].Detail, "espeak-ng") {
		t.Errorf("expected the surviving finding to be the phoneme one, got %+v", sounds[0])
	}
}

func TestHomoglyphLookAlike(t *testing.T) {
	if !hasKind(checkPair("G0LD", "GOLD"), "look-alike") {
		t.Error("expected look-alike for G0LD/GOLD")
	}
	if !hasKind(checkPair("Modern", "Modem"), "look-alike") {
		t.Error("expected look-alike for Modern/Modem (rn vs m)")
	}
	if hasKind(checkPair("Gold", "Silver"), "look-alike") {
		t.Error("Gold/Silver should not be look-alikes")
	}
}

func TestConfusableChars(t *testing.T) {
	if !hasKind(checkSingle("Sl0pe"), "confusable-chars") {
		t.Error("expected confusable-chars for Sl0pe")
	}
	if hasKind(checkSingle("Slope"), "confusable-chars") {
		t.Error("Slope has no confusable characters")
	}
}

func TestRhyme(t *testing.T) {
	if !hasKind(checkPair("GoldWing", "Sting"), "rhyme") {
		t.Error("expected rhyme for GoldWing/Sting")
	}
}

// A rhyme whose openings also match (same onset consonant) is promoted from a
// bare LOW rhyme to a MEDIUM sound-similar finding; a rhyme with a different
// opening stays a plain rhyme. Needs espeak-ng for the opening comparison.
func TestRhymeOpeningPromotion(t *testing.T) {
	if !phonetic.PhonemesAvailable() {
		t.Skip("espeak-ng not installed; the opening comparison needs the phoneme engine")
	}

	// "Hot Guy" / "HawkEye": both open on /h/ and rhyme on "-y". Their whole-word
	// distance is safe only because "Guy" carries an extra /g/, so this promotion
	// is the finding that catches them.
	hotGuy := checkPair("Hot Guy", "HawkEye")
	if !hasKind(hotGuy, "sound-similar") {
		t.Errorf("expected sound-similar for Hot Guy/HawkEye, got %+v", hotGuy)
	}
	if hasKind(hotGuy, "rhyme") {
		t.Errorf("Hot Guy/HawkEye should be promoted past a bare rhyme, got %+v", hotGuy)
	}

	// "Monsoon" / "Balloon" rhyme on "-oon" but open differently (/m/ vs /b/), so
	// they stay a plain rhyme and are not called sound-similar by this path.
	monsoon := checkPair("Monsoon", "Balloon")
	if !hasKind(monsoon, "rhyme") {
		t.Errorf("expected a bare rhyme for Monsoon/Balloon, got %+v", monsoon)
	}
	if hasKind(monsoon, "sound-similar") {
		t.Errorf("Monsoon/Balloon open differently and should not be sound-similar, got %+v", monsoon)
	}
}

func TestSyllableBounds(t *testing.T) {
	if !hasKind(checkSingle("Gold"), "too-few-syllables") { // 1 syllable
		t.Error("expected too-few-syllables for Gold")
	}
	if !hasKind(checkSingle("Indivisibility"), "too-many-syllables") { // 7 syllables
		t.Error("expected too-many-syllables for Indivisibility")
	}
	for _, ok := range []string{"GoldWing", "Tangerine", "Thunder"} { // 2–5 syllables
		if hasKind(checkSingle(ok), "too-few-syllables") || hasKind(checkSingle(ok), "too-many-syllables") {
			t.Errorf("%q should be within the 2–5 syllable range", ok)
		}
	}
}

func TestProfanityContains(t *testing.T) {
	// The swear word appears verbatim, including across a camelCase boundary.
	for _, c := range []string{"Shitstorm", "GoldFucker", "Cunt", "mother-fucker"} {
		issues := checkSingle(c)
		var crit *Issue
		for i := range issues {
			if issues[i].Kind == "profanity" {
				crit = &issues[i]
			}
		}
		if crit == nil {
			t.Errorf("expected a profanity issue for %q, got %+v", c, issues)
			continue
		}
		if crit.Severity != SevCritical {
			t.Errorf("expected CRITICAL profanity for %q, got %v", c, crit.Severity)
		}
	}
}

func TestProfanitySoundsLike(t *testing.T) {
	// "Phuck" is spelled differently but sounds like a swear word; both the
	// espeak-ng and Metaphone engines should catch it.
	if !hasKind(checkSingle("Phuck"), "profanity") {
		t.Error("expected profanity (sounds-like) for Phuck")
	}
}

func TestProfanityClean(t *testing.T) {
	for _, c := range []string{"GoldWing", "Thunder", "Tangerine", "Knight"} {
		if hasKind(checkSingle(c), "profanity") {
			t.Errorf("%q should not be flagged as profanity", c)
		}
	}
}

func TestProfanityAllowlist(t *testing.T) {
	// "Scunthorpe" contains "cunt" but is an allowlisted innocent word.
	if hasKind(checkSingle("Scunthorpe"), "profanity") {
		t.Error("Scunthorpe should be exempt from the profanity check (Scunthorpe problem)")
	}
}

func TestProwords(t *testing.T) {
	// A handle that is, or sounds like, a procedure word — including buried in a
	// compound handle, and via a respelling ("Brake" for "break").
	for _, c := range []string{"Roger", "Copy", "GoldBreak", "Brake"} {
		issues := checkSingle(c)
		if !hasKind(issues, "proword") {
			t.Errorf("expected a proword issue for %q, got %+v", c, issues)
			continue
		}
		for _, is := range issues {
			if is.Kind == "proword" && is.Severity != SevHigh {
				t.Errorf("expected HIGH proword for %q, got %v", c, is.Severity)
			}
		}
	}
}

func TestSafetyWords(t *testing.T) {
	for _, c := range []string{"Mayday", "Help", "Medic"} {
		issues := checkSingle(c)
		if !hasKind(issues, "safety-word") {
			t.Errorf("expected a safety-word issue for %q, got %+v", c, issues)
			continue
		}
		for _, is := range issues {
			if is.Kind == "safety-word" && is.Severity != SevCritical {
				t.Errorf("expected CRITICAL safety-word for %q, got %v", c, is.Severity)
			}
		}
	}
}

func TestProwordsClean(t *testing.T) {
	for _, c := range []string{"GoldWing", "Thunder", "Tangerine", "Playa",
		"Rover", "Scout", "Helper"} { // contain over/out/help only as substrings
		if hasKind(checkSingle(c), "proword") || hasKind(checkSingle(c), "safety-word") {
			t.Errorf("%q should not be flagged as a proword/safety word", c)
		}
	}
}

// TestProwordGlued checks that a proword glued into an all-lower-case run is
// caught the same as the camelCase spelling — detection must not depend on a
// handle's capitalization (the "break break" urgent proword said as one word).
func TestProwordGlued(t *testing.T) {
	for _, c := range []string{"BreakBreak", "Breakbreak", "breakbreak", "BREAKBREAK"} {
		if !hasKind(checkSingle(c), "proword") {
			t.Errorf("expected a proword issue for %q (glued 'break')", c)
		}
	}
}

func TestProwordAllowlist(t *testing.T) {
	// "Breakfast" embeds "break" but is an innocent word.
	if hasKind(checkSingle("Breakfast"), "proword") {
		t.Error("Breakfast should be exempt from the proword check")
	}
}

func TestNoFalsePositive(t *testing.T) {
	issues := checkPair("Thunder", "Playa")
	for _, is := range issues {
		if is.Severity >= SevMedium {
			t.Errorf("unexpected significant issue for Thunder/Playa: %+v", is)
		}
	}
}

func TestAnalyzeSorted(t *testing.T) {
	issues := Analyze([]string{"GoldWing", "goldwing", "golffoxtrot"})
	if len(issues) == 0 {
		t.Fatal("expected issues")
	}
	for i := 1; i < len(issues); i++ {
		if issues[i-1].Severity < issues[i].Severity {
			t.Error("issues not sorted by descending severity")
		}
	}
}

func TestCheckAgainst(t *testing.T) {
	issues := CheckAgainst("Nite", []string{"Knight", "GoldWing"})
	if len(issues) == 0 {
		t.Fatal("expected issues for Nite against the baseline")
	}
	// The candidate is always A, the baseline term is B; no baseline-vs-baseline
	// pairs should appear (e.g. Knight vs GoldWing).
	foundKnight := false
	for _, is := range issues {
		if is.A != "Nite" {
			t.Errorf("expected candidate Nite as A, got %+v", is)
		}
		if is.B == "Knight" {
			foundKnight = true
		}
	}
	if !foundKnight {
		t.Errorf("expected a conflict between Nite and Knight, got %+v", issues)
	}
	// Sorted most-severe first.
	for i := 1; i < len(issues); i++ {
		if issues[i-1].Severity < issues[i].Severity {
			t.Error("issues not sorted by descending severity")
		}
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"", "abc", 3},
		{"same", "same", 0},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestExpandDigits(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Dog4", "DogFour"},
		{"GoldWing-2Bar", "GoldWing-TwoBar"},
		{"K9", "KNine"},
		{"Echo", "Echo"},
		{"42", "FourTwo"},
	}
	for _, c := range cases {
		if got := expandDigits(c.in); got != c.want {
			t.Errorf("expandDigits(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDigitReadAsWord(t *testing.T) {
	// "Dog4" is spoken "Dog Four", so it collides with "DogFour".
	issues := checkPair("Dog4", "DogFour")
	if len(issues) != 1 || issues[0].Kind != "duplicate" {
		t.Errorf("expected Dog4/DogFour to be a duplicate, got %+v", issues)
	}
	// The shared spoken word "four" is found across the digit boundary.
	if !hasKind(checkPair("Dog4", "Cat4"), "shared-word") {
		t.Errorf("expected shared-word (four) for Dog4/Cat4, got %+v", checkPair("Dog4", "Cat4"))
	}
}

func TestTokens(t *testing.T) {
	got := tokens("GoldWing-2Bar")
	want := []string{"gold", "wing", "bar"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tokens = %v, want %v", got, want)
	}
}

func TestExpandInitialisms(t *testing.T) {
	// Separator-delimited single letters and fully-uppercase tokens are spelled
	// out as they are read aloud.
	if got := expandInitialisms("S A"); got != "Ess Ay" {
		t.Errorf(`expandInitialisms("S A") = %q, want "Ess Ay"`, got)
	}
	if got := expandInitialisms("LL"); got != "ElEl" {
		t.Errorf(`expandInitialisms("LL") = %q, want "ElEl"`, got)
	}
	if got := expandInitialisms("USB Key"); got != "YouEssBee Key" {
		t.Errorf(`expandInitialisms("USB Key") = %q, want "YouEssBee Key"`, got)
	}
	// A trailing/standalone capital is spelled out.
	if got := expandInitialisms("GoldX"); got != "GoldEx" {
		t.Errorf(`expandInitialisms("GoldX") = %q, want "GoldEx"`, got)
	}

	// An uppercase run that is the onset of an ordinary word is left untouched —
	// these are the glued mixed-case forms we deliberately do not guess at.
	if got := expandInitialisms("GoldWing"); got != "GoldWing" {
		t.Errorf(`expandInitialisms("GoldWing") = %q, want "GoldWing"`, got)
	}
	if got := expandInitialisms("GBush"); got != "GBush" {
		t.Errorf(`expandInitialisms("GBush") = %q, want "GBush"`, got)
	}
	if got := expandInitialisms("USBKey"); got != "USBKey" {
		t.Errorf(`expandInitialisms("USBKey") = %q, want "USBKey"`, got)
	}
	if got := expandInitialisms("Gold"); got != "Gold" {
		t.Errorf(`expandInitialisms("Gold") = %q, want "Gold"`, got)
	}
}

func TestSpokenFormInitialismThenDigits(t *testing.T) {
	// Initialisms are spelled out before digits expand, so a lone letter beside a
	// digit reads correctly.
	if got := spokenForm("R2D2"); got != "ArTwoDeeTwo" {
		t.Errorf(`spokenForm("R2D2") = %q, want "ArTwoDeeTwo"`, got)
	}
	if got := spokenForm("K9"); got != "KayNine" {
		t.Errorf(`spokenForm("K9") = %q, want "KayNine"`, got)
	}
	// An ordinary word with a digit is unaffected by the initialism pass.
	if got := spokenForm("Dog4"); got != "DogFour" {
		t.Errorf(`spokenForm("Dog4") = %q, want "DogFour"`, got)
	}
}

func TestSpelledLettersNoFalseContainment(t *testing.T) {
	// "S A" is spoken "ess ay", not the syllable "sa", so it is not contained in
	// "Tulsa"; likewise "LL" ("el el") is not contained in "NullSet".
	if hasKind(checkPair("S A", "Tulsa"), "substring") {
		t.Errorf("S A should not be 'contained' in Tulsa, got %+v", checkPair("S A", "Tulsa"))
	}
	if hasKind(checkPair("LL", "NullSet"), "substring") {
		t.Errorf("LL should not be 'contained' in NullSet, got %+v", checkPair("LL", "NullSet"))
	}
	// But a real spoken word inside another still counts as contained.
	if !hasKind(checkPair("Sun", "Sunfire"), "substring") {
		t.Errorf("expected Sun to be contained in Sunfire, got %+v", checkPair("Sun", "Sunfire"))
	}
}
