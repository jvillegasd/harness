package land

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func (f *fixture) merge(number int, head, text string, accepted bool) (Merged, error) {
	f.t.Helper()
	file := filepath.Join(f.t.TempDir(), "body.md")
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return Merge(f.env(), number, head, file, accepted)
}

func (f *fixture) merges() int {
	return len(slices.DeleteFunc(slices.Clone(f.calls), func(c string) bool { return !strings.HasPrefix(c, "pr merge") }))
}

func TestMerge(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	p.Title, p.Body = "docs: more", "## Why\n\nmore\n\n## Tradeoffs\n\n- a over b: b is slower\n\n## Evidence\n\n- ran\n"

	m, err := f.merge(1, p.HeadRefOid, "Adds more docs.\n", false)
	if err != nil || m.State != "MERGED" || m.MergeCommit != "abc123" {
		t.Fatalf("got %+v, %v", m, err)
	}
	var call string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "pr merge") {
			call = c
		}
	}
	if want := "pr merge 1 --squash --match-head-commit " + p.HeadRefOid + " --subject docs: more --body-file "; !strings.HasPrefix(call, want) || strings.Contains(call, "--delete-branch") {
		t.Errorf("merge call %q, want prefix %q and no --delete-branch", call, want)
	}
	if got, want := f.mergeBodies[0], "Adds more docs.\n\n## Tradeoffs\n\n- a over b: b is slower\n"; got != want {
		t.Errorf("body %q, want %q", got, want)
	}

	p.State = "OPEN"
	if _, err := f.merge(1, p.HeadRefOid, "Adds docs.\n\n## Tradeoffs\n\nNone\n", false); err != nil {
		t.Fatal(err)
	}
	if got := f.mergeBodies[1]; strings.Count(got, "## Tradeoffs") != 1 || !strings.Contains(got, "None") {
		t.Errorf("body %q repeats or replaces the given tradeoffs", got)
	}

	p.State, f.readBackFails = "OPEN", true
	if m, err := f.merge(1, p.HeadRefOid, "x\n", false); err == nil || m.State != "" {
		t.Errorf("a failed read-back got %+v, %v", m, err)
	}
	f.readBackFails = false

	p.State, f.mergeRefused = "OPEN", true
	if m, err := f.merge(1, p.HeadRefOid, "x\n", false); err == nil || m.State != "OPEN" {
		t.Errorf("a refused merge got %+v, %v", m, err)
	}
}

func TestMergeRefusals(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	refused := func(name, head, text string, accepted bool, want string) {
		t.Helper()
		before := f.merges()
		_, err := f.merge(1, head, text, accepted)
		if !errors.Is(err, ErrMergeRefused) || !strings.Contains(err.Error(), want) || f.merges() != before {
			t.Errorf("%s: got %v and %d merge calls, want a refusal mentioning %q and none", name, err, f.merges()-before, want)
		}
	}

	refused("empty body", p.HeadRefOid, " \n", false, "is empty")
	refused("moved head", "0000000", "x\n", false, "not the checked")
	p.IsDraft = true
	refused("draft", p.HeadRefOid, "x\n", true, "stop lane")
	p.IsDraft = false

	f.checks[1] = `[{"name":"ci","bucket":"pending"}]`
	f.checksErr[1] = errors.New("exit status 8")
	refused("pending check", p.HeadRefOid, "x\n", true, "held by the forge")
	delete(f.checks, 1)
	delete(f.checksErr, 1)

	f.verdict(1, "FAIL", p.HeadRefOid, "me")
	refused("failed verdict", p.HeadRefOid, "x\n", false, "--accepted")
	if m, err := f.merge(1, p.HeadRefOid, "x\n", true); err != nil || m.State != "MERGED" {
		t.Errorf("an accepted FAIL verdict got %+v, %v", m, err)
	}
}
