# TODO

- [ ] **Profanity / unfortunate-sounds check.** Flag callsigns that are swear
  words, or whose syllables sound like (or combine into) swear words —
  including across the camelCase/word boundaries and via the phonetic codes, so
  obfuscated or accidental spellings are caught too. Should run as a
  per-callsign check (likely HIGH severity). Needs a curated word list plus
  phonetic/substring matching; consider an allowlist for false positives
  (e.g. "Scunthorpe problem").
