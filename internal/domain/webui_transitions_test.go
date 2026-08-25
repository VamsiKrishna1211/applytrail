package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The browser client carries its own copy of the pipeline graph, because a
// board that offers a move the API rejects is worse than a board with no moves
// at all. A copy can drift, so this test reads the TypeScript back and compares
// it to the real thing.
//
// It parses rather than executes: running it would mean a Node toolchain in the
// Go test path, and the table is a literal precisely so it can be read as data.
const webUIStatusFile = "../../web/src/core/domain/status.ts"

var (
	transitionTable = regexp.MustCompile(`(?s)export const ALLOWED_TRANSITIONS[^{]*\{(.*?)\n\}`)
	transitionRow   = regexp.MustCompile(`^\s*(\w+):\s*\[([^\]]*)\]`)
	statusList      = regexp.MustCompile(`(?s)export const STATUSES = \[(.*?)\] as const`)
	quoted          = regexp.MustCompile(`'([^']+)'`)
)

func TestWebUITransitionTableMatchesDomain(t *testing.T) {
	source := readWebUIStatuses(t)

	block := transitionTable.FindStringSubmatch(source)
	require.Len(t, block, 2, "ALLOWED_TRANSITIONS is not a literal object any more; update this test or the UI")

	found := map[Status][]string{}
	for _, line := range strings.Split(block[1], "\n") {
		row := transitionRow.FindStringSubmatch(line)
		if row == nil {
			continue
		}
		targets := []string{}
		for _, match := range quoted.FindAllStringSubmatch(row[2], -1) {
			targets = append(targets, match[1])
		}
		found[Status(row[1])] = targets
	}

	require.Len(t, found, len(allowedTransitions), "the UI and the domain disagree on how many statuses exist")

	for status, allowed := range allowedTransitions {
		want := make([]string, 0, len(allowed))
		for target := range allowed {
			want = append(want, string(target))
		}
		slices.Sort(want)

		got, ok := found[status]
		require.Truef(t, ok, "the UI has no transitions for %q", status)
		slices.Sort(got)

		require.Equalf(t, want, got,
			"the UI offers different moves from %q than the service accepts:\n"+
				"  fix %s to match internal/domain/application.go", status, webUIStatusFile)
	}
}

func TestWebUIStatusListMatchesDomain(t *testing.T) {
	source := readWebUIStatuses(t)

	block := statusList.FindStringSubmatch(source)
	require.Len(t, block, 2, "STATUSES is not a literal array any more")

	got := []string{}
	for _, match := range quoted.FindAllStringSubmatch(block[1], -1) {
		got = append(got, match[1])
	}

	want := make([]string, len(AllStatuses))
	for i, status := range AllStatuses {
		want[i] = string(status)
	}

	// Order matters here: the funnel renders in this sequence, and it is meant
	// to be the pipeline's own order rather than an alphabetical accident.
	require.Equal(t, want, got, "the UI lists statuses in a different order than AllStatuses")
}

func readWebUIStatuses(t *testing.T) string {
	t.Helper()

	path := filepath.Clean(webUIStatusFile)
	source, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("no browser client checked out at %s", path)
	}
	require.NoError(t, err)
	return string(source)
}
