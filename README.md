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

For the strongest sound-alike detection, run it in Docker — the image bundles
`espeak-ng`, which enables phoneme-level comparison (see below):

```sh
docker build -t handlechecker .
docker run --rm handlechecker GoldWing GoldBar golffoxtrot Knight Nite Echo
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

The sound- and spelling-based checks run on the callsign's **spoken form** — how
it's read aloud. Digits become words (`Dog4` → "DogFour") and all-caps letter
runs are spelled out (`S A` → "ess ay", `USB Key` → "you ess bee key"), so a
spelled-out callsign isn't mistaken for a syllable — `S A` is "ess ay", not the
"-sa" in `Tulsa`. An uppercase run *attached to* a lowercase word is left as a
word, not guessed at (`GBush`, `USBKey` stay as written). The written-roster
checks (look-alike, confusable characters) instead use the literal glyphs.

Per callsign:
- **NATO concatenation** — the name is just NATO phonetic words strung together
  (`golffoxtrot` = golf + foxtrot), or is itself a NATO word (`Echo`).
- **Length** — too short to survive radio noise, or too long to say cleanly.
- **Syllable count** — fewer than 2 or more than 5 syllables (aim for 2–5, so
  the name is neither too curt nor too long-winded on the air).
- **Confusable characters** — contains glyphs easily misread on a roster
  (`Sl0pe` — the `0` reads as `O`).

Per pair:
- **Duplicate** — identical once case/punctuation/spacing are ignored.
- **Sound-alike** — the headline radio concern: different spelling, same sound.
  Two tiers, HIGH (near-identical) and MEDIUM (similar). When `espeak-ng` is
  available this uses a phoneme-level distance that accounts for vowel quality
  (`Knight`/`Nite` = HIGH, `Gold`/`Gild` = MEDIUM); otherwise it falls back to
  Metaphone 3 key matching.
- **Rhyme** — share the same final vowel sound (`Sting` / `GoldWing` → "ing").
- **Edit distance** — differ by only one or two letters (`Spark` / `Sparc`).
- **Look-alike (written roster)** — identical once confusable characters are
  folded together (`G0LD` / `GOLD`, `Modern` / `Modem` via `rn`↔`m`).
- **Shared word** — share a whole word token, including camelCase splits
  (`GoldWing` / `GoldBar` both contain "Gold").
- **Common prefix / suffix** — share an opening or closing run of letters.
- **Substring** — one callsign's spoken form is wholly contained in the other
  (`Sun` inside `Sunfire`).

## Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--min` | `info` | Minimum severity to print (`info`/`low`/`medium`/`high`/`critical`). |
| `--fail-on` | `high` | Exit non-zero if any issue at this severity or above is found (`never` to always exit 0). |
| `--no-color` | `false` | Disable ANSI colors. |

The non-zero exit on `--fail-on` makes it easy to gate a callsign roster in a
script or CI check.

## How "sounds alike" works

The phonetics live in `internal/phonetic`, with two engines:

1. **Phoneme distance (preferred, needs `espeak-ng`).** `espeak-ng` converts
   each callsign — including invented words — to a phoneme sequence, which is
   compared with a feature-weighted edit distance: substituting two similar
   sounds (e.g. /b/↔/p/, or `Gold`↔`Cold`) costs little, while different sounds
   (and different *vowels*) cost more. This is the only tier that models vowel
   quality, so `Gold`/`Gild` come out as merely *similar* rather than identical.
   Thresholds were tuned against a battery of real pronunciations.
2. **Metaphone 3 (fallback, pure Go).** When `espeak-ng` isn't installed, the
   tool uses Metaphone 3 key matching, run with and without vowel positions to
   grade near-identical vs. consonant-only matches. Note the whole Metaphone
   family collapses all vowels to one value, so it captures vowel *position* but
   not vowel *identity* (`Gold` and `Gild` match) — which is exactly why the
   phoneme tier is preferred when available.

**Rhyme** compares the final vowel sound and **SyllableCount** estimates
cadence; these run alongside Levenshtein edit distance, word-token analysis, and
a homoglyph fold (for written rosters).

## Dependencies

- **Go 1.26+** and one Go module:
  [`github.com/dlclark/metaphone3`](https://github.com/dlclark/metaphone3) — a
  BSD-3 licensed port of Lawrence Philips' Metaphone 3 (v2.1.3, released under
  BSD-3 via OpenRefine).
- **`espeak-ng`** — *optional* runtime dependency. If present on `PATH` it
  enables the phoneme-level sound check; if absent, the tool degrades gracefully
  to the Metaphone 3 fallback. The provided Docker image bundles it.

Everything else (rhyme, syllable counting, homoglyph folding, edit distance,
phoneme feature distance, NATO decomposition) is pure standard library.
```
