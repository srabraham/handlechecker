# TODO

- [ ] **Profanity / unfortunate-sounds check.** Flag callsigns that are swear
  words, or whose syllables sound like (or combine into) swear words —
  including across the camelCase/word boundaries and via the phonetic codes, so
  obfuscated or accidental spellings are caught too. Should run as a
  per-callsign check (likely HIGH severity). Needs a curated word list plus
  phonetic/substring matching; consider an allowlist for false positives
  (e.g. "Scunthorpe problem").

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
