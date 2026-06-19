package checker

import (
	"strings"
	"testing"
)

// hasKind reports whether any issue involving the given subjects has the kind.
func hasKind(issues []Issue, kind string) bool {
	for _, is := range issues {
		if is.Kind == kind {
			return true
		}
	}
	return false
}

func TestNatoConcatenation(t *testing.T) {
	issues := checkSingle("golffoxtrot")
	if !hasKind(issues, "nato-concatenation") {
		t.Fatalf("expected nato-concatenation for golffoxtrot, got %+v", issues)
	}
}

func TestSingleNatoWord(t *testing.T) {
	if !hasKind(checkSingle("Tango"), "nato-word") {
		t.Error("expected Tango to be flagged as a NATO word")
	}
}

func TestNatoDecompose(t *testing.T) {
	if w, ok := natoDecompose("golffoxtrot"); !ok || strings.Join(w, " ") != "golf foxtrot" {
		t.Errorf("natoDecompose(golffoxtrot) = %v, %v", w, ok)
	}
	if _, ok := natoDecompose("goldwing"); ok {
		t.Error("goldwing should not decompose into NATO words")
	}
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

func TestSyllableCadence(t *testing.T) {
	if !hasKind(checkPair("Catherine", "Tangerine"), "syllable-count") {
		t.Error("expected syllable-count note for two 3-syllable callsigns")
	}
	if hasKind(checkPair("Gold", "Wing"), "syllable-count") {
		t.Error("short callsigns should not trigger syllable-count note")
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

func TestTokens(t *testing.T) {
	got := tokens("GoldWing-2Bar")
	want := []string{"gold", "wing", "bar"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tokens = %v, want %v", got, want)
	}
}
