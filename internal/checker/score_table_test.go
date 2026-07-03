package checker

import (
	"strings"
	"testing"

	"github.com/srabraham/handlechecker/internal/phonetic"
)

// TestSoundScoreTable is a tuning diagnostic, not an assertion: it logs every
// corpus pair's raw sound signals, per-signal contributions, and combined
// score (run with -v). Use it when retuning the weights/ramps in score.go —
// the pass/fail judgment lives in TestConfusabilityCorpus.
func TestSoundScoreTable(t *testing.T) {
	if !phonetic.PhonemesAvailable() {
		t.Skip("espeak-ng not installed; the combined score needs the phoneme engine")
	}
	t.Logf("%-12s %-12s %-13s | %5s %5s %5s %3s %5s %4s %4s | %5s %5s %5s %5s %5s | %5s %s",
		"A", "B", "label", "dist", "cont", "ovD", "syl", "onset", "rime", "ctr",
		"c.gl", "c.cn", "c.ov", "c.en", "c.st", "total", "band")
	for _, p := range loadCorpus(t) {
		sa, sb := spokenForm(p.a), spokenForm(p.b)
		na, nb := normalize(sa), normalize(sb)
		substr := strings.Contains(na, nb) || strings.Contains(nb, na)
		sig, ok := gatherSoundSignals(sa, sb, substr)
		if !ok {
			t.Logf("%-12s %-12s: espeak-ng could not phonemize", p.a, p.b)
			continue
		}
		c, total := scoreSignals(sig)
		band := "-"
		switch {
		case total >= scoreHigh:
			band = "HIGH"
		case total >= scoreMed:
			band = "MED"
		case total >= scoreLow && sig.rime != "":
			band = "low"
		}
		label := "distinct"
		if p.confusable {
			label = "confusable"
		}
		rime := "-"
		if sig.rime != "" {
			rime = "y"
		}
		ctr := "-"
		if sig.contour != "" {
			ctr = sig.contour
		}
		t.Logf("%-12s %-12s %-13s | %5.2f %5.2f %5.2f %3d %5.2f %4s %4s | %5.2f %5.2f %5.2f %5.2f %5.2f | %5.2f %s",
			p.a, p.b, label, sig.dist, sig.contain, sig.ovDist, sig.ovSyl, sig.onset, rime, ctr,
			c.global, c.contain, c.overlap, c.ends, c.contour, total, band)
	}
}
