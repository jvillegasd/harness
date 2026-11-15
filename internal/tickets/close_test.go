package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const issuesRel = "docs/specs/export/issues"

func localFeature(t *testing.T, statuses map[int]string, edges map[int][]int) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, issuesRel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs/specs/export/spec.md"), []byte("# Export\n\nStatus: in-progress\n\n## Problem\n\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for n, st := range statuses {
		blocked := "none"
		if bs := edges[n]; len(bs) > 0 {
			var parts []string
			for _, b := range bs {
				parts = append(parts, fmt.Sprintf("%02d", b))
			}
			blocked = strings.Join(parts, ", ")
		}
		body := fmt.Sprintf("# %02d Slice %d\n\nStatus: %s\n\n## What to build\n\nx\n\n## Acceptance criteria\n\nx\n\n## Seam\n\nx\n\n## Blocked by\n\n%s\n\n## Out of scope\n\nx\n", n, n, st, blocked)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%02d-slice.md", n)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func closeRun(t *testing.T, root string, merged ...int) []string {
	t.Helper()
	ts, problems, err := Check(root, issuesRel)
	if err != nil || len(problems) > 0 {
		t.Fatalf("check: %v %v", problems, err)
	}
	c := Config{Tracker: "local-markdown", Labels: map[string]string{"blocked": "waiting", "ready_for_agent": "go"}}
	changed, err := Close(c, root, issuesRel, ts, merged)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

func statusOf(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	_, s := status(strings.Split(string(raw), "\n"))
	return s
}

func TestCloseAdvancesTheFrontier(t *testing.T) {
	root := localFeature(t,
		map[int]string{1: "in-progress", 2: "waiting", 3: "waiting", 4: "in-progress"},
		map[int][]int{2: {1}, 3: {1, 4}, 4: {1}})
	got := closeRun(t, root, 1)
	want := []string{issuesRel + "/01-slice.md", issuesRel + "/02-slice.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changed %v, want %v", got, want)
	}
	for n, st := range map[int]string{1: "done", 2: "go", 3: "waiting", 4: "in-progress"} {
		if s := statusOf(t, root, fmt.Sprintf("%s/%02d-slice.md", issuesRel, n)); s != st {
			t.Errorf("ticket %d is %q, want %q", n, s, st)
		}
	}
	if s := statusOf(t, root, "docs/specs/export/spec.md"); s != "in-progress" {
		t.Errorf("spec became %q with tickets still open", s)
	}
	if again := closeRun(t, root, 1); len(again) != 0 {
		t.Errorf("a second run changed %v", again)
	}
}

func TestCloseFinishesTheParent(t *testing.T) {
	root := localFeature(t, map[int]string{1: "done", 2: "in-progress"}, map[int][]int{2: {1}})
	got := closeRun(t, root, 2)
	if want := []string{issuesRel + "/02-slice.md", "docs/specs/export/spec.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed %v, want %v", got, want)
	}
	if s := statusOf(t, root, "docs/specs/export/spec.md"); s != "done" {
		t.Errorf("spec is %q, want done", s)
	}
}

func TestSetStatusAddsAMissingLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01-x.md")
	if err := os.WriteFile(path, []byte("# 01 X\n\n## What to build\n\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := setStatus(path, "done"); !ok || err != nil {
		t.Fatalf("setStatus = %v, %v", ok, err)
	}
	raw, _ := os.ReadFile(path)
	if want := "# 01 X\n\nStatus: done\n\n## What to build\n\nx\n"; string(raw) != want {
		t.Errorf("got %q, want %q", raw, want)
	}
}
