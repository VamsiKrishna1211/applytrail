package dedupe_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/VamsiKrishna1211/applytrail/internal/dedupe"
)

func TestTitleSimilarity(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		wantMin float64
		wantMax float64
		why     string
	}{
		{
			name: "identical after normalisation", a: "senior software engineer", b: "senior software engineer",
			wantMin: 1, wantMax: 1,
		},
		{
			name: "word order does not matter", a: "backend engineer", b: "engineer backend",
			wantMin: 1, wantMax: 1,
			why: "titles are bags of qualifiers, which is why this is a set measure",
		},
		{
			name: "one extra qualifier", a: "senior software engineer", b: "senior software engineer backend",
			wantMin: 0.7, wantMax: 0.8,
			why: "0.75 lands in the review band: it may be a team variant rather than the same job",
		},
		{
			name: "unrelated roles", a: "senior software engineer", b: "warehouse forklift operator",
			wantMin: 0, wantMax: 0.1,
		},
		{
			name: "shared generic word only", a: "software engineer", b: "sales engineer",
			wantMin: 0.3, wantMax: 0.35,
			why: "sharing 'engineer' is not enough to be the same job",
		},
		{name: "empty input", a: "", b: "senior engineer", wantMin: 0, wantMax: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupe.TitleSimilarity(tc.a, tc.b)
			assert.GreaterOrEqual(t, got, tc.wantMin, tc.why)
			assert.LessOrEqual(t, got, tc.wantMax, tc.why)
		})
	}
}

func TestDifferingTokens(t *testing.T) {
	diff := dedupe.DifferingTokens("senior software engineer", "senior software engineer payments")
	assert.Equal(t, []string{"payments"}, diff,
		"the differing word is what makes a borderline verdict reviewable")
}
