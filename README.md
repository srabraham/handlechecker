# handlechecker

A command-line tool for vetting proposed Burning Man radio callsigns. Give it a
list of candidate callsigns and it flags pairs (and individual names) that are
likely to be confused on the air — by spelling, by sight, and, most importantly,
by **how they sound**.

## Install / run

```sh
go run ./cmd/handlecheckercli GoldWing GoldBar golffoxtrot Knight Nite Echo
# or build a binary:
go build -o handlecheckercli ./cmd/handlecheckercli
./handlecheckercli GoldWing GoldBar golffoxtrot Knight Nite Echo
```

Example output:

```
[HIGH]     Knight / Nite — sound nearly identical (same Metaphone key)
[HIGH]     golffoxtrot — is just NATO phonetic words strung together (golf + foxtrot)
[MEDIUM]   Echo — is itself a NATO phonetic word ("echo")
[MEDIUM]   GoldWing / GoldBar — both contain the word "gold"
```

## What it checks

Each callsign is checked on its own, and every **pair** is compared against
every other. Findings are ranked CRITICAL → HIGH → MEDIUM → LOW → INFO.

Per callsign:
- **NATO concatenation** — the name is just NATO phonetic words strung together
  (`golffoxtrot` = golf + foxtrot), or is itself a NATO word (`Echo`).
- **Length** — too short to survive radio noise, or too long to say cleanly.

- **Confusable characters** — contains glyphs easily misread on a roster
  (`Sl0pe` — the `0` reads as `O`).

Per pair:
- **Duplicate** — identical once case/punctuation/spacing are ignored.
- **Sound-alike** — matching **Double Metaphone** code (e.g. `Knight` / `Nite`,
  `Phipps` / `Fips`), or same Soundex code. This is the headline radio concern:
  different spelling, same sound. Double Metaphone cross-matches primary and
  secondary pronunciations, so it handles names with multiple valid readings.
- **Rhyme** — share the same final vowel sound (`Sting` / `GoldWing` → "ing").
- **Edit distance** — differ by only one or two letters (`Spark` / `Sparc`).
- **Look-alike (written roster)** — identical once confusable characters are
  folded together (`G0LD` / `GOLD`, `Modern` / `Modem` via `rn`↔`m`).
- **Shared word** — share a whole word token, including camelCase splits
  (`GoldWing` / `GoldBar` both contain "Gold").
- **Common prefix / suffix** — share an opening or closing run of letters.
- **Substring** — one callsign is wholly contained in the other.
- **Cadence** — same syllable count for longer callsigns (INFO level).

## Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--min` | `info` | Minimum severity to print (`info`/`low`/`medium`/`high`/`critical`). |
| `--fail-on` | `high` | Exit non-zero if any issue at this severity or above is found (`never` to always exit 0). |
| `--no-color` | `false` | Disable ANSI colors. |

The non-zero exit on `--fail-on` makes it easy to gate a callsign roster in a
script or CI check.

## How "sounds alike" works

The phonetics live in `internal/phonetic`:
- **Double Metaphone** (the primary engine) models English pronunciation and
  emits two codes per word, so `Knight`, `Nite`, and `Night` collapse together
  and alternate pronunciations cross-match.
- **Soundex** is a coarser fallback for looser similarities.
- **Rhyme** compares the final vowel sound; **SyllableCount** estimates cadence.

These run alongside Levenshtein edit distance, word-token analysis, and a
homoglyph fold (for written rosters) so that look-alike and sound-alike problems
are both surfaced.

## Dependencies

Go 1.26+, and a single external module:
[`github.com/antzucaro/matchr`](https://github.com/antzucaro/matchr) for its
well-tested Double Metaphone implementation. Everything else (Soundex, rhyme,
syllable counting, homoglyph folding, edit distance, NATO decomposition) is
pure standard library.
```
