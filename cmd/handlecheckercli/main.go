// Command handlecheckercli checks a set of proposed Burning Man radio
// callsigns for ways they might be confused with one another — by spelling,
// by sight, and especially by how they sound on the air.
//
// Usage:
//
//	handlecheckercli GoldWing GoldBar golffoxtrot Echo
//
// It exits non-zero when any issue at or above the --fail-on severity is
// found, so it can be used as a gate in scripts.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/srabraham/handlechecker/internal/checker"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		minSev  = flag.String("min", "info", "minimum severity to report: info|low|medium|high|critical")
		failOn  = flag.String("fail-on", "high", "exit non-zero if any issue at this severity or above is found (or 'never')")
		noColor = flag.Bool("no-color", false, "disable colored output")
		debug   = flag.Bool("debug", false, "print the phonemes used for each callsign (to stderr)")
		explain = flag.Bool("explain", false, "diagnose exactly two callsigns: report what each individual check concluded, fired or not")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] CALLSIGN [CALLSIGN ...]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s --explain [flags] CALLSIGN_A CALLSIGN_B\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Checks proposed radio callsigns for confusability (spelling, sight, and sound).")
		fmt.Fprintln(os.Stderr, "\nFlags:")
		flag.PrintDefaults()
	}
	flag.Parse()

	callsigns := flag.Args()
	if len(callsigns) == 0 {
		flag.Usage()
		return 2
	}

	color := !*noColor && isTerminal(os.Stdout)

	if *explain {
		if len(callsigns) != 2 {
			fmt.Fprintf(os.Stderr, "--explain needs exactly two callsigns, got %d\n", len(callsigns))
			return 2
		}
		printExplain(callsigns[0], callsigns[1], color)
		return 0
	}

	minLevel, ok := parseSeverity(*minSev)
	if !ok {
		fmt.Fprintf(os.Stderr, "invalid --min value %q\n", *minSev)
		return 2
	}
	failLevel, failNever := checker.SevCritical, false
	if strings.EqualFold(*failOn, "never") {
		failNever = true
	} else if failLevel, ok = parseSeverity(*failOn); !ok {
		fmt.Fprintf(os.Stderr, "invalid --fail-on value %q\n", *failOn)
		return 2
	}

	if *debug {
		printPhonemeDebug(callsigns)
	}

	issues := checker.Analyze(callsigns)

	shown := 0
	worst := checker.SevInfo
	for _, is := range issues {
		if is.Severity > worst {
			worst = is.Severity
		}
		if is.Severity < minLevel {
			continue
		}
		shown++
		printIssue(is, color)
	}

	fmt.Println()
	if shown == 0 {
		fmt.Printf("Checked %d callsign(s): no issues at or above %s.\n",
			len(callsigns), strings.ToUpper(*minSev))
	} else {
		fmt.Printf("Checked %d callsign(s): %d issue(s) shown.\n", len(callsigns), shown)
	}

	if !failNever && len(issues) > 0 && worst >= failLevel {
		return 1
	}
	return 0
}

// printPhonemeDebug writes each callsign's phoneme breakdown to stderr, so it
// stays out of the (potentially parsed) findings on stdout.
func printPhonemeDebug(callsigns []string) {
	if !checker.PhonemesAvailable() {
		fmt.Fprintln(os.Stderr,
			"debug: espeak-ng not installed; sound checks use the Metaphone 3 fallback (no phonemes to show)")
		fmt.Fprintln(os.Stderr)
		return
	}
	fmt.Fprintln(os.Stderr, "debug: phonemes via espeak-ng (digits read as words):")
	for _, d := range checker.DebugPhonemes(callsigns) {
		name := d.Callsign
		if d.Spoken != d.Callsign {
			name = fmt.Sprintf("%s → %s", d.Callsign, d.Spoken)
		}
		phon := strings.Join(d.Phonemes, " ")
		if phon == "" {
			phon = "(no phonemes)"
		}
		fmt.Fprintf(os.Stderr, "  %-24s %s\n", name, phon)
	}
	fmt.Fprintln(os.Stderr)
}

// printExplain runs every pairwise check on the two callsigns and prints what
// each one concluded — fired (with its severity) or silent (with why) — so a
// non-match can be understood, not just trusted.
func printExplain(a, b string, color bool) {
	fmt.Printf("Explaining %s vs %s\n\n", bold(color, a), bold(color, b))

	if checker.PhonemesAvailable() {
		for _, d := range checker.DebugPhonemes([]string{a, b}) {
			name := d.Callsign
			if d.Spoken != d.Callsign {
				name = fmt.Sprintf("%s → %s", d.Callsign, d.Spoken)
			}
			phon := strings.Join(d.Phonemes, " ")
			if phon == "" {
				phon = "(no phonemes)"
			}
			fmt.Printf("  %-26s [%s]\n", name, phon)
		}
	} else {
		fmt.Println("  (espeak-ng not installed — sound checks use the Metaphone 3 fallback)")
	}
	fmt.Println()

	exps := checker.ExplainPair(a, b)
	nameW := 0
	for _, e := range exps {
		if len(e.Name) > nameW {
			nameW = len(e.Name)
		}
	}

	fired := 0
	worst := checker.SevInfo
	for _, e := range exps {
		var tag string
		if e.Fired {
			fired++
			if e.Severity > worst {
				worst = e.Severity
			}
			plain := fmt.Sprintf("[%s]", e.Severity)
			tag = plain
			if color {
				tag = colorize(e.Severity, plain)
			}
			tag += strings.Repeat(" ", max(0, 10-len(plain)))
		} else {
			dash := "    —     "
			if color {
				dash = ansiGray + dash + ansiReset
			}
			tag = dash
		}
		fmt.Printf("%s%-*s  %s\n", tag, nameW, e.Name, e.Detail)
	}

	fmt.Println()
	if fired == 0 {
		fmt.Println("No check flagged this pair.")
	} else {
		fmt.Printf("%d check(s) flagged this pair; most severe: %s.\n", fired, worst)
	}
}

func printIssue(is checker.Issue, color bool) {
	plain := fmt.Sprintf("[%s]", is.Severity)
	pad := strings.Repeat(" ", max(0, 10-len(plain)))
	tag := plain
	if color {
		tag = colorize(is.Severity, plain)
	}
	subject := bold(color, is.A)
	if is.B != "" {
		subject += " / " + bold(color, is.B)
	}
	fmt.Printf("%s%s %s — %s\n", tag, pad, subject, is.Detail)
}

func parseSeverity(s string) (checker.Severity, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "info":
		return checker.SevInfo, true
	case "low":
		return checker.SevLow, true
	case "medium", "med":
		return checker.SevMedium, true
	case "high":
		return checker.SevHigh, true
	case "critical", "crit":
		return checker.SevCritical, true
	}
	return checker.SevInfo, false
}

// --- terminal styling --------------------------------------------------------

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGray   = "\033[90m"
)

func colorize(sev checker.Severity, s string) string {
	var c string
	switch sev {
	case checker.SevCritical, checker.SevHigh:
		c = ansiRed
	case checker.SevMedium:
		c = ansiYellow
	case checker.SevLow:
		c = ansiCyan
	default:
		c = ansiGray
	}
	return c + s + ansiReset
}

func bold(color bool, s string) string {
	if !color {
		return s
	}
	return ansiBold + s + ansiReset
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
