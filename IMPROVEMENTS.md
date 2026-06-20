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

---

## This pass

Implementing the top three: **proword/safety-word check** (§1),
**phoneme-aware Rhyme/SyllableCount** (§2), and **Undo in the web app** (§3).
