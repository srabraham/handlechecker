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
