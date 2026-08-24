package dedupe

import "strings"

// TitleSimilarity scores how alike two normalized titles are, on [0, 1].
//
// The measure is Jaccard overlap of token sets rather than an edit distance,
// for two reasons. Job titles are bags of qualifiers whose order varies
// ("Engineer, Backend" and "Backend Engineer" are the same role), and a set
// overlap is explainable: the engine can tell a caller which words differed,
// which an edit distance cannot.
//
// Both inputs are expected to have been through NormalizeTitle already.
func TitleSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}

	setA := tokenSet(a)
	setB := tokenSet(b)
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}

	intersection := 0
	for token := range setA {
		if setB[token] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// DifferingTokens returns the tokens present in one title but not the other,
// which is what turns a bare score into a readable explanation.
func DifferingTokens(a, b string) []string {
	setA := tokenSet(a)
	setB := tokenSet(b)

	var diff []string
	for token := range setA {
		if !setB[token] {
			diff = append(diff, token)
		}
	}
	for token := range setB {
		if !setA[token] {
			diff = append(diff, token)
		}
	}
	return diff
}

func tokenSet(s string) map[string]bool {
	fields := strings.Fields(s)
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		set[f] = true
	}
	return set
}
