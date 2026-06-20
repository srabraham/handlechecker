# TODO

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
    battery — expand the battery and tune more rigorously.
  - Use stress/syllable boundaries from espeak (currently stripped) to align
    syllables and weight stressed nuclei more heavily.
  - Consider `libespeak-ng` via cgo to avoid one subprocess per callsign (the
    phonemize cache already amortizes this to one call per callsign).
