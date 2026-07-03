# TODO

See `IMPROVEMENTS.md` for the broader prioritized critique and roadmap.

- [ ] **Handle input & word segmentation.** `expandInitialisms`
  (`internal/checker/initialisms.go`) spells out an all-caps run only when it is
  *not* glued to a following lowercase word, so genuinely ambiguous glued forms
  are left untouched: `USBKey` is read as one word, not "USB Key", and `GBush`
  is not read as "Gee Bush". Resolving these needs to know where the word breaks
  are, which the input doesn't currently carry. Think about how handles are
  entered — e.g. let submitters mark spacing/segmentation explicitly (`USB Key`,
  `Gold-Wing`), or add a smarter "acronym + TitleWord" tokenizer split
  (`HTMLParser` → `HTML` + `Parser`) — and weigh its new failure modes
  (`USBkey` → `US` + `Bkey`) before adopting it. Until then, glued mixed-case is
  deliberately not guessed at.

- [x] **Procedure-word / safety-word check.** Implemented in
  `internal/checker/prowords.go` (`checkProwords`, wired into `checkSingle`):
  flags callsigns that are or sound like a radio procedure word (Roger, Copy,
  Break, Over, Out, … — HIGH) or an emergency/distress word (Mayday, Pan-Pan,
  Help, Fire, Medic, … — CRITICAL), matching each spoken word token exactly or
  by ear (espeak distance, Metaphone fallback), plus a substring pass over the
  glued handle for the distinctive words so detection doesn't depend on
  capitalization ("Breakbreak" == "BreakBreak"), with a Scunthorpe-style
  allowlist ("Breakfast"). Possible refinements: multi-word prowords
  ("say again"), and extending substring coverage to the short words
  ("over"/"out") if a safe boundary heuristic is found.

- [x] **Phoneme-aware prosody.** `Rhyme` and `SyllableCount`
  (`internal/phonetic/prosody.go`) now derive from the espeak-ng phoneme stream
  when available (falling back to the spelling heuristic otherwise), so silent
  letters and digraphs no longer fool them. Syllable counting adds an extra
  syllable for wide triphthong tokens ("aI@") that espeak fuses, so "Playa" /
  "Fire" aren't undercounted.

- [x] **Profanity / unfortunate-sounds check.** Implemented in
  `internal/checker/profanity.go` (`checkProfanity`, wired into `checkSingle`):
  a hardcoded swear-word list (Carlin's seven dirty words plus other
  unambiguous profanities/slurs) matched by substring across camelCase/word
  boundaries and by sound (espeak-ng phoneme distance, Metaphone 3 fallback) so
  phonetic respellings like "Phuck" are caught too. Any hit is CRITICAL. A
  `profanityAllowlist` exempts the well-known innocent collisions
  ("Scunthorpe problem"). Possible refinements: cross-token phonetic matching
  (a swear spanning a camelCase boundary by sound, not just spelling), and
  leetspeak folding tuned for profanity (1->i) distinct from the roster
  homoglyph map (1->l).

- [x] **Phoneme-level sound similarity.** Implemented in `internal/phonetic`
  (`phonemes.go`, `features.go`): `espeak-ng` G2P → feature-weighted phoneme edit
  distance, used as the preferred sound-alike engine with Metaphone 3 as the
  fallback when espeak-ng is absent. Bundled in the Docker image. Remaining
  refinements:
  - Diphthongs are approximated by a single feature vector; model them as a
    two-vowel trajectory for more accurate distances.
  - Feature weights and the HIGH/MED thresholds were hand-tuned against a small
    battery — a labeled confusability corpus with precision/recall floors now
    guards them (`internal/checker/testdata/confusability.tsv`,
    `TestConfusabilityCorpus`); keep growing it with real pairs.
  - Stress is now kept and used (stressed nuclei weighted, unstressed indels
    discounted, vowel↔consonant substitutions floored — see `phonemes.go`);
    full syllable-level alignment remains a possible future refactor.
  - Consider `libespeak-ng` via cgo to avoid one subprocess per callsign (the
    phonemize cache already amortizes this to one call per callsign).
