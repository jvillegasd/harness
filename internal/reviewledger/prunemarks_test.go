package reviewledger

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestPruneMarksOnlyAnOpenPullRequestsDirectory(t *testing.T) {
	forge := func(args ...string) ([]byte, error) {
		switch args[2] {
		case "1":
			return []byte(`{"state":"OPEN"}`), nil
		case "2":
			return []byte(`{"state":"MERGED"}`), nil
		case "3":
			return []byte(`{"state":"CLOSED"}`), nil
		}
		return nil, errors.New("HTTP 502")
	}
	base := filepath.Join("x", "branches")
	dirs := []string{filepath.Join(base, "pr-1"), filepath.Join(base, "pr-2"), filepath.Join(base, "pr-3"), filepath.Join(base, "pr-4"), filepath.Join(base, "feat%2Fgone")}
	marks := PruneMarks(dirs, forge)
	want := map[string]string{dirs[0]: "pull request #1 is open", dirs[3]: "forge state unavailable"}
	if len(marks) != len(want) {
		t.Fatalf("marks = %v", marks)
	}
	for d, m := range want {
		if marks[d] != m {
			t.Errorf("%s marked %q, want %q", d, marks[d], m)
		}
	}
}
