package worktree

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func fakeForge(pulls map[string]string) Forge {
	return func(args ...string) ([]byte, error) {
		if len(args) < 4 || args[0] != "pr" || args[1] != "list" || args[2] != "--head" {
			return nil, fmt.Errorf("unexpected gh %v", args)
		}
		out, ok := pulls[args[3]]
		if !ok {
			return []byte("[]"), nil
		}
		if out == "" {
			return nil, errors.New("HTTP 502")
		}
		return []byte(out), nil
	}
}

func TestReadPullNamesTheMergedHeadThatRetiresAnUnlandedBranch(t *testing.T) {
	dir := repo(t)
	origin := filepath.Join(filepath.Dir(dir), "origin.git")
	run(t, dir, "init", "-q", "--bare", origin)
	run(t, dir, "remote", "add", "origin", origin)
	path := branchTree(t, dir, "feat/merged")
	head := run(t, path, "rev-parse", "HEAD")
	run(t, dir, "push", "-q", "origin", head+":refs/pull/7/head")

	squash(t, dir, "feat/merged")
	write(t, filepath.Join(dir, "a.txt"), "rewritten\n")
	run(t, dir, "commit", "-q", "-am", "rewrite")
	s, err := Inspect(tree(t, dir, path), "main")
	if err != nil || s.State != "unlanded" {
		t.Fatalf("setup: state %q, %v", s.State, err)
	}

	p := ReadPull(dir, s, fakeForge(map[string]string{"feat/merged": `[{"number":7,"state":"MERGED","headRefOid":"` + head + `"}]`}))
	if p.Err != nil || p.Into != head || p.String() != "#7 MERGED" || p.Held() {
		t.Fatalf("pull = %+v (%s)", p, p)
	}
	if _, err := Retire(dir, path, Options{Into: p.Into}); err != nil {
		t.Fatalf("retire --into the merged head: %v", err)
	}
	if exists(path) || hasBranch(t, dir, "feat/merged") {
		t.Error("the worktree or its branch survived")
	}
}

func TestReadPullMarksOpenNoneAndUnavailable(t *testing.T) {
	dir := repo(t)
	forge := fakeForge(map[string]string{
		"feat/open":  `[{"number":3,"state":"CLOSED","headRefOid":"x"},{"number":8,"state":"OPEN","headRefOid":"y"}]`,
		"feat/flaky": "",
	})
	for branch, want := range map[string]string{"feat/open": "#8 OPEN held", "feat/none": "none", "feat/flaky": "unavailable"} {
		s, err := Inspect(tree(t, dir, branchTree(t, dir, branch)), "main")
		if err != nil {
			t.Fatal(err)
		}
		before := s.State
		p := ReadPull(dir, s, forge)
		if p.String() != want || p.Into != "" || s.State != before {
			t.Errorf("%s: %q into %q, state %q", branch, p, p.Into, s.State)
		}
	}
	if p := ReadPull(dir, Status{}, forge); p.String() != "none" {
		t.Errorf("a detached head read %q", p)
	}
}
