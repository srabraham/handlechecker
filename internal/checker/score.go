package checker

// This file combines the pairwise sound-similarity signals into a single
// confusability score, replacing the former cascade of independent threshold
// gates (global distance gated on a clean shared run, overlap gated on a
// syllable floor, rhyme promoted only with a matching onset, …). Each signal is
// converted to a continuous contribution in [0,1] and the contributions are
// combined with a noisy-OR:
//
//	total = 1 − Π(1 − cᵢ)
//
// rather than a plain sum, so evidence accumulates — three near-misses add up
// instead of producing silence — but *redundant* evidence saturates: a rhyme
// adds little to a pair whose global distance already says "nearly identical"
// (the rime is part of that global match), which a linear sum would
// double-count. The total is banded into severities at the very end
// (scoreHigh/scoreMed/scoreLow), and the reported kind/detail comes from the
// top contributing signal, so the output vocabulary (sound-alike,
// sound-similar, sound-overlap, sound-substring, rhyme) is unchanged.
//
// Tuning: the weights and ramps below were hand-tuned against the labeled
// corpus (testdata/confusability.tsv, TestConfusabilityCorpus) and the pair
// battery in checker_test.go. TestSoundScoreTable logs every corpus pair's
// signals, contributions, and total — start there when retuning.

import (
	"fmt"

	"github.com/srabraham/handlechecker/internal/phonetic"
)

// soundSignals are the raw per-pair sound measurements, gathered once and then
// scored. All distances are normalized 0..1 (0 = identical).
type soundSignals struct {
	dist    float64 // global phoneme distance (PhoneticDistance)
	contain float64 // containment edge distance (PhoneticContainment); 1 = none
	ovSyl   int     // syllables spanned by the best shared run (PhoneticOverlap)
	ovDist  float64 // distance within that run; 1 = no run
	rime    string  // shared rime key; "" when the rimes differ (Rhyme)
	onset   float64 // onset-consonant distance (SharedOpening); 1 = unshared
	contour string  // shared stress contour (StressContour); "" when they differ
}

// gatherSoundSignals measures every sound signal for the spoken forms sa and
// sb. ok is false when espeak-ng is unavailable (or either side cannot be
// phonemized), in which case the caller falls back to Metaphone 3.
//
// substr reports that the spelled substring check already fired; the
// containment signal is zeroed then, because a spelled containment necessarily
// sounds contained too and the substring finding already explains it.
func gatherSoundSignals(sa, sb string, substr bool) (soundSignals, bool) {
	s := soundSignals{contain: 1, ovDist: 1, onset: 1}
	d, ok := phonetic.PhoneticDistance(sa, sb)
	if !ok {
		return s, false
	}
	s.dist = d
	if c, ok := phonetic.PhoneticContainment(sa, sb); ok && !substr {
		s.contain = c
	}
	if syl, od, ok := phonetic.PhoneticOverlap(sa, sb); ok {
		s.ovSyl, s.ovDist = syl, od
	}
	if ra, rb := phonetic.Rhyme(sa), phonetic.Rhyme(sb); ra != "" && ra == rb && len(ra) >= 2 {
		s.rime = ra
	}
	if od, ok := phonetic.SharedOpening(sa, sb); ok {
		s.onset = od
	}
	if ca, aok := phonetic.StressContour(sa); aok {
		if cb, bok := phonetic.StressContour(sb); bok && ca == cb {
			s.contour = ca
		}
	}
	return s, true
}

// Signal weights and ramps. Each signal contributes weight × closeness, where
// closeness ramps linearly from 1 at distance 0 to 0 at the signal's zero
// point — so every hard threshold of the old cascade becomes a slope, and a
// pair just past a former cutoff loses contribution gradually instead of
// vanishing.
const (
	// Global phoneme distance: the primary signal. The zero point sits where the
	// old MED cutoff's rationale ran out (clearly-different pairs measure ≥ 0.3:
	// Thunder/Lantern 0.26, GoldWing/GoldBar 0.42). Reference closenesses:
	// Gold/Cold 0.02 → 0.93, Gold/Gild 0.13 → 0.55, Tulsa/Minty 0.21 → 0.27.
	weightGlobal = 1.0
	zeroGlobal   = 0.29

	// Containment: one callsign's whole pronunciation heard at an edge of the
	// other (the spoken "substring"). Genuine containments measure ≈ 0.00 and
	// near-misses (a single substituted edge sound) ≥ 0.22 — a wide gap — so the
	// weight alone puts an exact containment in the HIGH band and the ramp zeroes
	// out well before the near-misses ("Thunder" in "Plunder" 0.22, "DustyDog" in
	// "ADustyLog" 0.38).
	weightContain = 0.95
	zeroContain   = 0.15

	// Best shared run of sounds (local alignment), scaled by how many syllables
	// it spans (full weight at overlapFullSyl). A clean three-syllable run
	// ("Dusty Dog"/"A Dusty Log", run distance 0.08) carries the finding almost
	// on its own; a clean shorter run is a corroborating nudge (this replaces
	// the old similarOverlapMax confirmation gate: Gold/Gild keep a modest
	// boost, while diffuse pairs with no clean run — HawkEye/Fowler 0.20,
	// Tulsa/Minty 0.22 — get almost none). The full-weight floor stays at 3
	// syllables for the same reason the old gate's was: two-syllable runs align
	// deceptively cleanly on short words ("Tulsa" vs the "Delta" of
	// "Delta Victor", 0.07) without being confusable.
	weightOverlap  = 0.55
	zeroOverlap    = 0.25
	overlapFullSyl = 3

	// Alike at both ends: a shared rime is a fixed contribution, and a matching
	// onset (opening consonants) adds more, because sharing both ends leaves only
	// the middle to tell the words apart ("Hot Guy"/"HawkEye"). A rhyme alone
	// (weightRhyme) lands in the LOW band by itself, mirroring the old bare-rhyme
	// finding; rhyme + onset can reach MED with only mild help from the rest.
	weightRhyme = 0.30
	weightOnset = 0.25
	zeroOnset   = 0.15

	// Same stress contour (syllable count + stress placement): a weak
	// corroborating signal — the prosodic envelope survives channel noise, so
	// two handles with the same rhythm are that little bit easier to mistake.
	// Only counted for contours of 2+ syllables; every pair of stressed
	// monosyllables trivially matches.
	weightContour = 0.06
)

// Severity bands for the combined score. LOW additionally requires a shared
// rime — the only LOW-band sound finding we report is the plain rhyme, as
// before; diffuse sub-MED evidence without a rhyme stays silent.
//
// Calibration (2026-07 corpus measurements): the nearly-identical pairs score
// ≥ 0.91 (Gold/Cold 0.97, Merlin/Marlin 0.92) and the borderline-MED ones just
// under (Pickle/Nickel 0.85), so HIGH sits between. Genuine MED catches score
// 0.56–0.84 (Thunder/Plunder 0.56, DustyDog/ADustyLog 0.61, Gold/Gild 0.60)
// while the closest distinct pairs stay under 0.47 (Monsoon/Balloon 0.46, a
// rhyme that correctly stays LOW; then Tulsa/Delta Victor 0.37) — MED at 0.53
// splits the gap. Known noise: Sweet/Swat 0.81 (documented in the corpus —
// coarse binary vowel features); known sound miss: Coyote/Peyote 0.52, just
// under the band, whose MEDIUM verdict is carried by the edit-distance check
// instead.
const (
	scoreHigh = 0.86
	scoreMed  = 0.53
	scoreLow  = 0.25
)

// closeness converts a normalized distance to a contribution factor: 1 at
// distance 0, falling linearly to 0 at the zero point.
func closeness(dist, zero float64) float64 {
	if dist >= zero {
		return 0
	}
	return (zero - dist) / zero
}

// soundContributions are the weighted per-signal contributions to the combined
// score, kept separately so the top contributor can name the finding and so
// --explain can show each signal's share.
type soundContributions struct {
	global, contain, overlap, ends, contour float64
}

// scoreSignals converts raw signals to weighted contributions and combines
// them (noisy-OR) into the total confusability score.
func scoreSignals(s soundSignals) (soundContributions, float64) {
	var c soundContributions
	c.global = weightGlobal * closeness(s.dist, zeroGlobal)
	c.contain = weightContain * closeness(s.contain, zeroContain)
	sylFrac := float64(s.ovSyl) / overlapFullSyl
	if sylFrac > 1 {
		sylFrac = 1
	}
	c.overlap = weightOverlap * closeness(s.ovDist, zeroOverlap) * sylFrac
	if s.rime != "" {
		c.ends = weightRhyme + weightOnset*closeness(s.onset, zeroOnset)
	}
	if len(s.contour) >= 2 {
		c.contour = weightContour
	}
	total := 1 - (1-c.global)*(1-c.contain)*(1-c.overlap)*(1-c.ends)*(1-c.contour)
	return c, total
}

// soundVerdict is the combined-score outcome for a pair: the measured signals,
// their contributions, the total, and — when the total reaches a band — the
// single sound finding to report.
type soundVerdict struct {
	sig     soundSignals
	contrib soundContributions
	total   float64
	fired   bool
	sev     Severity // meaningful only when fired
	kind    string
	detail  string
}

// scoreSound measures and scores the pair, then bands the total into at most
// one sound finding whose kind/detail names the top contributing signal. ok is
// false when espeak-ng is unavailable.
func scoreSound(sa, sb string, substr bool) (soundVerdict, bool) {
	sig, ok := gatherSoundSignals(sa, sb, substr)
	if !ok {
		return soundVerdict{}, false
	}
	v := soundVerdict{sig: sig}
	v.contrib, v.total = scoreSignals(sig)

	switch {
	case v.total >= scoreMed:
		v.fired = true
		v.sev = SevMedium
		if v.total >= scoreHigh {
			v.sev = SevHigh
		}
		v.kind, v.detail = describeTopSignal(v)
	case v.total >= scoreLow && sig.rime != "":
		// Below MED the only finding worth reporting is the plain rhyme.
		v.fired = true
		v.sev = SevLow
		v.kind = "rhyme"
		v.detail = fmt.Sprintf("rhyme (both end with the %q sound)", sig.rime)
	}
	return v, true
}

// describeTopSignal names the finding after the largest contribution.
func describeTopSignal(v soundVerdict) (kind, detail string) {
	c := v.contrib
	top := c.global
	kind = "sound-alike"
	if v.sev < SevHigh {
		kind = "sound-similar"
	}
	detail = fmt.Sprintf("sound %s (phoneme distance %.2f, confusability %.2f via espeak-ng)",
		map[bool]string{true: "nearly identical", false: "similar"}[v.sev >= SevHigh], v.sig.dist, v.total)

	if c.contain > top {
		top = c.contain
		kind = "sound-substring"
		detail = fmt.Sprintf("one sounds like the whole of the other (contained pronunciation, edge distance %.2f, confusability %.2f via espeak-ng)",
			v.sig.contain, v.total)
	}
	if c.overlap > top {
		top = c.overlap
		kind = "sound-overlap"
		detail = fmt.Sprintf("share a %d-syllable run of sounds (run distance %.2f, confusability %.2f via espeak-ng); easily confused on the air",
			v.sig.ovSyl, v.sig.ovDist, v.total)
	}
	if c.ends > top {
		kind = "sound-similar"
		if v.sig.onset <= zeroOnset {
			detail = fmt.Sprintf("open alike and rhyme (both end with the %q sound, confusability %.2f via espeak-ng); easily confused on the air",
				v.sig.rime, v.total)
		} else {
			detail = fmt.Sprintf("rhyme and otherwise sound close (both end with the %q sound, confusability %.2f via espeak-ng)",
				v.sig.rime, v.total)
		}
	}
	return kind, detail
}

// pairSound is every sound-related decision for a pair, computed once and
// consumed by both checkPair (to emit issues) and ExplainPair (to narrate
// them) — the two cannot drift because there is only one decision.
type pairSound struct {
	verdict        soundVerdict
	espeakOK       bool
	msev           Severity
	mkind          string
	mdetail        string
	mok            bool
	metaphoneFired bool
	verdictFired   bool
	fallbackRhyme  bool   // plain rhyme finding on the Metaphone-fallback path
	rime           string // matched rime key ("" when none), either engine
	strongSound    bool   // a MEDIUM+ sound finding fired (suppresses suffix)
}

// evaluateSound runs the combined score (espeak-ng) and Metaphone 3 for the
// pair and settles which findings fire. Metaphone is always consulted and
// surfaced only when it warns more strongly than the combined verdict, so the
// two engines never emit duplicate findings; when espeak-ng is unavailable it
// is the only sound engine, plus the spelling-heuristic rhyme.
func evaluateSound(sa, sb string, substr bool) pairSound {
	var ps pairSound
	ps.verdict, ps.espeakOK = scoreSound(sa, sb, substr)
	ps.msev, ps.mkind, ps.mdetail, ps.mok = metaphoneSound(sa, sb)

	verdictSev := Severity(-1)
	if ps.espeakOK && ps.verdict.fired {
		ps.verdictFired = true
		verdictSev = ps.verdict.sev
		ps.rime = ps.verdict.sig.rime
	} else if ps.espeakOK {
		ps.rime = ps.verdict.sig.rime
	}

	ps.metaphoneFired = ps.mok && ps.msev > verdictSev
	// A strong Metaphone match explains a plain rhyme the same way a strong
	// phoneme match does — drop the redundant LOW.
	if ps.verdictFired && ps.verdict.sev == SevLow && ps.metaphoneFired && ps.msev >= SevMedium {
		ps.verdictFired = false
	}

	if !ps.espeakOK {
		// Metaphone-fallback path: the spelling-heuristic rhyme, plain LOW only.
		if ra, rb := phonetic.Rhyme(sa), phonetic.Rhyme(sb); ra != "" && ra == rb && len(ra) >= 2 {
			ps.rime = ra
			if !(ps.metaphoneFired && ps.msev >= SevMedium) {
				ps.fallbackRhyme = true
			}
		}
	}

	ps.strongSound = (ps.verdictFired && ps.verdict.sev >= SevMedium) ||
		(ps.metaphoneFired && ps.msev >= SevMedium)
	return ps
}
