package checker

import "strings"

// natoWords is the NATO phonetic alphabet plus the most common spelling
// variants. Callsigns built out of these words are a problem: on the radio
// they collide directly with how operators spell things out.
var natoWords = map[string]bool{
	"alfa": true, "alpha": true,
	"bravo":   true,
	"charlie": true,
	"delta":   true,
	"echo":    true,
	"foxtrot": true,
	"golf":    true,
	"hotel":   true,
	"india":   true,
	"juliett": true, "juliet": true,
	"kilo":     true,
	"lima":     true,
	"mike":     true,
	"november": true,
	"oscar":    true,
	"papa":     true,
	"quebec":   true,
	"romeo":    true,
	"sierra":   true,
	"tango":    true,
	"uniform":  true,
	"victor":   true,
	"whiskey":  true, "whisky": true,
	"xray": true, "x-ray": true,
	"yankee": true,
	"zulu":   true,
}

// natoDecompose attempts to split the normalized string s entirely into NATO
// phonetic-alphabet words. It returns the sequence of words and true if the
// whole string is exactly a concatenation of NATO words (e.g. "golffoxtrot"
// -> ["golf","foxtrot"]). The match is greedy-with-backtracking so it finds a
// valid split when one exists.
func natoDecompose(s string) ([]string, bool) {
	s = strings.ToLower(s)
	if s == "" {
		return nil, false
	}
	// Longest NATO word is "november"/"juliett" at 8 chars.
	const maxWord = 8
	memo := make(map[int][]string)
	resolved := make(map[int]bool)

	var solve func(i int) ([]string, bool)
	solve = func(i int) ([]string, bool) {
		if i == len(s) {
			return []string{}, true
		}
		if resolved[i] {
			return memo[i], memo[i] != nil
		}
		resolved[i] = true
		for l := 1; l <= maxWord && i+l <= len(s); l++ {
			word := s[i : i+l]
			if !natoWords[word] {
				continue
			}
			if rest, ok := solve(i + l); ok {
				memo[i] = append([]string{word}, rest...)
				return memo[i], true
			}
		}
		memo[i] = nil
		return nil, false
	}

	return solve(0)
}

// leadingNatoWord returns the longest NATO word that s starts with, if any.
func leadingNatoWord(s string) string {
	s = strings.ToLower(s)
	best := ""
	for w := range natoWords {
		if strings.HasPrefix(s, w) && len(w) > len(best) {
			best = w
		}
	}
	return best
}
