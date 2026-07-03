# Improvement plan

A prioritized critique of the handle checker (engine + web app), captured for
reference. Status keys: **[doing]** in this pass, **[ ]** proposed, **[x]** done.

## 1. Missing domain checks (highest value)

- **[x] Procedure-word / safety-word collisions.** A callsign that *is* or
  *sounds like* a radio proword or an emergency word is dangerous: it gets
  parsed as procedure, not identity, and emergency words can trigger a real
  response or be misheard during an incident.
  - Prowords (HIGH): Roger, Copy, Wilco, Affirmative/Affirm, Negative,
    Disregard, Standby, Correction, Break, Over, Out.
  - Safety/distress (CRITICAL): Mayday, Pan-Pan, Securité, Help, Fire, Medic,
    Emergency, Evac, Rescue.
  - Same machinery as `profanity.go`: per-callsign, substring/token + sound
    match, espeak distance with Metaphone fallback.

- **[ ] Restore or remove the NATO / spelling-on-air check.** `nato.go` was
  deleted but CLAUDE.md still documented it; the dead doc reference was dropped
  2026-07. What remains open is the feature itself: when a callsign is unclear
  it is *spelled* on air, so confusability of the spelled-out form is a
  distinct axis from how the word sounds — decide whether to build that check
  for real.

- **[ ] Weight first-syllable distinctiveness over shared prefixes.** The start
  of a transmission is what gets clipped (PTT/VOX lag, squelch tail), so
  callsigns differing only at the *end* are the dangerous ones; a shared onset
  matters less than the symmetric prefix/suffix treatment implies.

- **[ ] Number discipline ("niner").** Digits expand to "Nine" etc.; radio
  convention is "niner"/"tree"/"fife". Pairs differing only by a spoken digit
  deserve a closer look than generic edit distance gives.

## 2. Engine accuracy

- **[x] Rhyme and SyllableCount should use the phoneme stream when espeak is
  present.** They are currently spelling-only, so silent letters and digraphs
  (Through/Cough) are mis-rhymed and the 2–5 syllable gate can be wrong. Derive
  both from the espeak phonemes when available; fall back to the heuristic
  otherwise (and when a phoneme token is unrecognized, so a partial reading is
  never trusted).

- **[ ] Metaphone fallback over-fires on vowels.** `EncodeVowels` collapses all
  vowels, so without espeak (local/CI) `Sheet`/`Shit`-type pairs read as
  sound-alikes — including a CRITICAL false positive in the profanity path.

- **[ ] Test the preferred engine without the binary.** Every espeak test
  `t.Skip`s when espeak-ng is absent, so CI never exercises the distance logic.
  Capture golden phoneme token sequences as fixtures and test
  `phonemeEditDistance`/`featureDistance` directly.

- **[ ] `phonemeCache` grows unbounded** over a long-lived web-server process.

## 3. Web app

- **[x] Undo.** Removing Skip left no way to recover from a misclicked
  Approve/Reject, and an accidental Approve permanently contaminates `existing`
  (and thus every later check). Add an "Undo last decision" that pops the
  decision, restores `existing`, and steps back into review — reachable from
  both the review pane and the summary (the only correction path for the last
  item now that "back to review" is gone).

- **[ ] Record *why* each handle was rejected/approved** (store the worst issue
  with the decision) so the summary is auditable.

- **[ ] Surface the pronunciation** (`checker.DebugPhonemes`) so reviewers can
  see how a handle is sounded out, especially digit/leet handles.

- **[ ] Gate approving a CRITICAL** behind a confirm.

- **[ ] Keyboard shortcuts (A/R), ARIA live-region banner, autofocus** for fast,
  accessible triage.

- **[ ] Session import/export (JSON)** so a committee can hand off / merge.

- **[ ] Tests for the `/api/check` handler** (source tagging, worst-rank).

## 4. Architecture / maintainability

- **[ ] Externalize the word lists** (profanity, prowords/safety, allowlists)
  via `go:embed` text files so non-developers can tune them.

- **[x] Reconcile doc drift** in CLAUDE.md (`nato.go`, "Reserved terms") —
  last stale reference (`nato.go`) dropped 2026-07.

## 5. Sound-similarity engine, round two (July 2026 review)

A deeper review of the phonetic engine asking: how do we get *better still* at
knowing when two handles actually sound similar on the air? Prioritized; the
first item is the prerequisite that makes the rest safe to do.

- **[x] Labeled confusability corpus with precision/recall assertions.**
  Implemented: `internal/checker/testdata/confusability.tsv` (ground-truth
  confusable/distinct pairs — seeded from the pairs cited in tests and comments
  plus fresh ones, including a documented known false positive, Sweet/Swat) and
  `TestConfusabilityCorpus` (`corpus_test.go`), which scores `checkPair`'s
  user-facing verdict (any finding ≥ MEDIUM) over the whole corpus and asserts
  precision/recall floors (baseline 2026-07: precision 0.97, recall 1.00 over
  49 pairs; floors 0.95/0.98, sized so one new misclassification fails). Engine
  changes are now judged by their effect on the whole corpus, not pair-by-pair.
  Growth path: the web app's approve/reject workflow is a free labeling
  pipeline if decisions are logged.

- **[x] Perceptual (channel-aware) substitution costs.** Implemented in
  `features.go`: `articDiff`'s uniform Hamming count is replaced by `articDist`,
  a weighted sum with per-feature perceptual weights modeling a band-limited
  noisy radio channel (Miller & Nicely 1955) — place of articulation and
  stridency cheap (0.5; the channel destroys those cues, so /p/–/t/, /m/–/n/,
  /f/–/θ/ now read as close), nasality/manner/voicing expensive (1.25–1.75;
  those cues survive noise), vowel-geometry features unchanged at 1.0 (graded
  vowel distances are a separate item below). This fixed the two inversions
  cited in the review: /p/–/t/ (0.063) is now cheaper than /p/–/b/ (0.079), and
  /f/–/θ/ halved to 0.095. The weights sum to ~15.75 so the distance scale (and
  the tuned thresholds) held: the corpus stayed at precision 0.97 / recall 1.00
  with no threshold changes, and two channel-confusion pairs (Pony/Tony,
  Fret/Threat) were added to it as regression anchors. `TestPerceptualWeights`
  (in `phonemes_test.go`) pins the orderings; stale comment-cited distances in
  `checker.go`/CLAUDE.md were re-measured via `--explain` and updated.

- **[x] Use stress — `parsePhonemes` previously stripped it.** Implemented:
  stress marks are kept as a `'` prefix on the vowel token (secondary stress
  folds into primary, since espeak demotes a word's stress when it is embedded
  in a longer handle — "CCS" vs "CCEssay"; a mark on a consonant carries
  forward to the syllable's vowel). `featureDistance` then (a) scales a vowel
  swap by `vowelWeight` only when a *stressed* nucleus is involved — two
  unstressed (reduced) vowels compare unscaled — and charges a mild
  `stressMismatchCost` for a stress shift; (c) `indelCost` discounts an
  unstressed-vowel indel (`unstressedIndelCost` 0.5), which moved
  "Blaze"/"Belize" from 0.225 (a hair under the 0.24 MED cutoff) to 0.125.
  A `syllabicityFloor` (0.5) on vowel↔consonant substitutions was needed
  alongside: the cheap unstressed indel otherwise opened a degenerate
  alignment ("insert the unstressed vowel, substitute vowel-for-consonant")
  that collapsed "Thunder"/"Lantern" into the MED band — the corpus caught it.
  Rime keys strip stress so "Sting" still rhymes with "Nesting". Corpus held
  at precision 0.97 / recall 1.00 with no threshold changes. Sub-item (b) —
  matching syllable-count + stress contour as a pairwise similarity signal —
  is deferred to the combined-score item below, where it belongs as one more
  continuous feature rather than another standalone gate.

- **[x] Combine evidence into one score; retire the threshold-gate cascade.**
  Implemented in `internal/checker/score.go`: `gatherSoundSignals` measures the
  continuous signals (global phoneme distance, containment edge distance, best
  shared run distance + syllables, shared rime + onset distance, and the
  stress contour — the signal deferred from the stress item above, via the new
  `phonetic.StressContour`), `scoreSignals` converts each to a weighted
  contribution through a linear closeness ramp (every old hard threshold is now
  a slope) and combines them with a **noisy-OR** rather than a plain sum — so
  evidence accumulates (Thunder/Plunder: moderate global 0.33 + clean shared
  run 0.30 + matching contour 0.06 → 0.56 MEDIUM, where each gate alone said
  nothing) but *redundant* evidence saturates (a rhyme adds little when the
  global distance already says near-identical, which a linear sum would
  double-count). The total is banded into severities at the end, the finding's
  kind/detail names the top contributing signal (output vocabulary unchanged),
  and `Issue.Score` carries the total for ranking. `evaluateSound` makes every
  sound decision (including the Metaphone-escalation and suppression rules) in
  one place consumed by both `checkPair` and `ExplainPair`, dissolving the old
  keep-in-lockstep-by-hand problem for the sound checks; `--explain` now shows
  each signal's measured value and contribution plus the banded total. Corpus
  held at precision 0.97 / recall 1.00 (tuning notes in `score.go`;
  `TestSoundScoreTable` logs the full per-pair signal/score table for future
  retuning). One known trade noted in the band comment: Coyote/Peyote's sound
  score lands just under the MED band (0.52 vs 0.53) — its MEDIUM verdict is
  carried by the edit-distance check instead.

Smaller, concrete gaps:

- **[ ] Word-level comparison for multi-word handles.** "Dusty Dog" vs
  "Rusty Hog": each word is a near-sound-alike of its counterpart, but the
  glued-string distance dilutes and the overlap check needs a shared run. A
  word-order swap ("DogDusty"/"DustyDog") is invisible to sequence alignment yet
  highly confusable in recall. `tokens` already decomposes handles: run the
  phoneme distance per word-pair (best bipartite matching), order-independent.
- **[ ] Weak-coda discount only fires at the very end of the glued sequence.**
  In "HotDog"-style compounds the word-final /t/ is interior and pays full
  price; apply the discount at each word's coda by keeping word boundaries
  through phonemization.
- **[ ] Pronunciation ambiguity for invented spellings.** espeak picks one
  reading of "Cyko"/"Phyre"; humans vary. Phonemize with a second voice (en-gb)
  or perturb ambiguous graphemes and compare the closest pronunciation pair, so
  an alternate reading can't hide a conflict.
- **[ ] Make the good engine always available.** Locally everything silently
  runs on Metaphone (different tool, different findings) and half the test suite
  `t.Skip`s. Embed a pure-Go G2P (CMUdict lookup per token + letter-to-sound
  fallback for OOV) so the phoneme engine always runs — or at minimum print
  which engine is active.
- **[ ] Graded vowel distances.** Binary height/backness rates "bit"/"bet",
  "beat"/"bit", "bet"/"bat" all identically (~0.13). Give vowels scalar
  positions in F1/F2 (formant) space and use Euclidean distance; diphthongs as
  start→end trajectories.

Considered and rejected: full TTS → noise → ASR round-trip simulation. It is
the theoretical ceiling for "do these actually sound alike" but heavy, slow,
non-deterministic, and hard to threshold; confusion-weighted substitution costs
capture most of the value at zero runtime cost.

---

## This pass

Implemented §5 item 4: the **combined confusability score** (noisy-OR over
continuous signal contributions, banded at the end), including the deferred
stress-contour signal, with `checkPair` and `ExplainPair` now sharing one
decision path (`evaluateSound`). Also closed the §4 doc-drift item. Next up
from §5's smaller gaps: word-level comparison for multi-word handles, or
making the good engine always available (embedded G2P).

Previous passes: labeled confusability corpus, perceptual channel-aware
substitution costs, stress-aware distances (§5 items 1–3); proword/safety-word
check (§1), phoneme-aware Rhyme/SyllableCount (§2), Undo in the web app (§3).
