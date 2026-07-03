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
  deleted but CLAUDE.md still documents it. When a callsign is unclear it is
  *spelled* on air, so confusability of the spelled-out form is a distinct axis
  from how the word sounds. Either bring the check back or drop the doc
  reference (currently dead documentation).

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

- **[ ] Reconcile doc drift** in CLAUDE.md (`nato.go`, "Reserved terms").

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

- **[ ] Use stress — `parsePhonemes` currently strips it.** Stress pattern is
  one of the strongest cues in noisy-channel word recognition. Keep espeak's
  stress marks, attach them to the following vowel, then: (a) scale
  substitution/indel cost by the stress of the syllable it occurs in (unstressed
  vowels reduce toward schwa and carry little identity — `vowelWeight` should
  not price a stressed and an unstressed nucleus swap the same); (b) treat a
  matching syllable-count + stress contour as a similarity signal (the
  principled version of the disabled `syllable-count` check); (c) discount
  schwa/unstressed-vowel indels ("Blaze"/"Belize" epenthesis) — the same root
  cause the `codaIndelCost` hack patches for trailing stops.

- **[ ] Combine evidence into one score; retire the threshold-gate cascade.**
  `checkPair` is a decision tree of eight tuned constants where each false
  positive/negative got its own gate (MED band gated on overlap, overlap gated
  on 3+ syllables, rhyme promoted only with matching onset…). Each patch is
  locally justified but the gates interact and every hard threshold is a cliff
  (0.239 is MEDIUM, 0.241 is invisible). Instead: compute the signals already
  available — global distance, overlap distance/syllables, containment edge
  distance, onset distance, rhyme, stress match, syllable-count delta — as
  continuous features, combine into one confusability score (hand-weighted
  linear to start; fit against the corpus once it's big enough), and band into
  severities at the end. Evidence then *accumulates* (three near-misses add up
  instead of producing silence), and tuning becomes "adjust weights, check
  precision/recall". Keep kind/detail by reporting the top contributing signals.

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

Implemented the **labeled confusability corpus** (§5, item 1) and **perceptual
channel-aware substitution costs** (§5, item 2). Next up: stress (§5, item 3).

Previous pass: proword/safety-word check (§1), phoneme-aware
Rhyme/SyllableCount (§2), Undo in the web app (§3).
