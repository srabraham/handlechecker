package checker

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/srabraham/handlechecker/internal/phonetic"
)

// This file evaluates the pairwise checks against the labeled corpus in
// testdata/confusability.tsv. Unlike the unit tests around it, it is an
// aggregate evaluation, not a set of per-pair assertions: each pair carries a
// ground-truth confusable/distinct label, the whole corpus is scored, and the
// test asserts precision/recall floors. That way a threshold or engine change
// is judged by its effect on the whole corpus — one pair may regress while five
// improve — instead of failing the first hard-coded pair it disagrees with.
// Known engine misses stay in the corpus, labeled by ground truth; the floors
// below are set from measured performance and updated deliberately.

// corpusPair is one labeled line of the corpus.
type corpusPair struct {
	a, b       string
	confusable bool
	line       int
}

// loadCorpus parses testdata/confusability.tsv.
func loadCorpus(t *testing.T) []corpusPair {
	t.Helper()
	f, err := os.Open("testdata/confusability.tsv")
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	defer f.Close()

	var pairs []corpusPair
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Split(text, "\t")
		if len(fields) != 3 {
			t.Fatalf("corpus line %d: want 3 tab-separated fields, got %d: %q", line, len(fields), text)
		}
		var confusable bool
		switch fields[0] {
		case "confusable":
			confusable = true
		case "distinct":
			confusable = false
		default:
			t.Fatalf("corpus line %d: unknown label %q", line, fields[0])
		}
		pairs = append(pairs, corpusPair{a: fields[1], b: fields[2], confusable: confusable, line: line})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	return pairs
}

// flaggedConfusable is the decision boundary the corpus is scored against: the
// pair counts as flagged when any pairwise finding reaches MEDIUM or above.
// This is deliberately the user-facing verdict rather than "a sound-* finding
// fired", because checkPair's suppression layering intentionally lets a
// spelling finding stand in for a sound one (e.g. Ranger/Stranger reports the
// HIGH spelled substring and suppresses the equivalent sound-substring).
func flaggedConfusable(a, b string) (bool, []Issue) {
	issues := checkPair(a, b)
	for _, is := range issues {
		if is.Severity >= SevMedium {
			return true, issues
		}
	}
	return false, issues
}

// Precision/recall floors for the corpus, with espeak-ng available. Measured
// performance when the floors were last set (2026-07, 28 confusable / 21
// distinct pairs): precision 0.97, recall 1.00. The one known false positive is
// Sweet/Swat (Metaphone's vowel-collapsing escalation plus the coarse binary
// vowel features — commented in the corpus file); there are no known misses.
// The floors sit between the measured values and the effect of one new
// misclassification, so at this corpus size a single genuine regression fails
// the test while the known noise passes. When adding known-miss or known-noise
// pairs, recompute and adjust these deliberately — the -v output prints the
// measured values.
const (
	corpusMinPrecision = 0.95
	corpusMinRecall    = 0.98
)

// TestConfusabilityCorpus scores checkPair against every labeled pair in the
// corpus and asserts the precision/recall floors above. Run with -v to see the
// measured metrics and every misclassified pair.
func TestConfusabilityCorpus(t *testing.T) {
	if !phonetic.PhonemesAvailable() {
		t.Skip("espeak-ng not installed; the corpus floors are calibrated for the phoneme engine")
	}
	pairs := loadCorpus(t)
	if len(pairs) == 0 {
		t.Fatal("corpus is empty")
	}

	var tp, fp, fn, tn int
	for _, p := range pairs {
		flagged, issues := flaggedConfusable(p.a, p.b)
		switch {
		case p.confusable && flagged:
			tp++
		case p.confusable && !flagged:
			fn++
			t.Logf("MISS (false negative, line %d): %q / %q labeled confusable but nothing >= MEDIUM fired; findings: %s",
				p.line, p.a, p.b, describeIssues(issues))
		case !p.confusable && flagged:
			fp++
			t.Logf("NOISE (false positive, line %d): %q / %q labeled distinct but flagged; findings: %s",
				p.line, p.a, p.b, describeIssues(issues))
		default:
			tn++
		}
	}

	precision := 1.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	recall := 1.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	t.Logf("corpus: %d pairs (%d confusable, %d distinct) — tp=%d fp=%d fn=%d tn=%d precision=%.2f recall=%.2f",
		len(pairs), tp+fn, fp+tn, tp, fp, fn, tn, precision, recall)

	if precision < corpusMinPrecision {
		t.Errorf("precision %.2f below floor %.2f — the checker is flagging pairs labeled distinct (see NOISE lines above)",
			precision, corpusMinPrecision)
	}
	if recall < corpusMinRecall {
		t.Errorf("recall %.2f below floor %.2f — the checker is missing pairs labeled confusable (see MISS lines above)",
			recall, corpusMinRecall)
	}
}

// describeIssues renders a findings list compactly for the miss/noise logs.
func describeIssues(issues []Issue) string {
	if len(issues) == 0 {
		return "(none)"
	}
	parts := make([]string, len(issues))
	for i, is := range issues {
		parts[i] = fmt.Sprintf("%s %s", is.Severity, is.Kind)
	}
	return strings.Join(parts, ", ")
}
