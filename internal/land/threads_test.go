package land

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (f *fixture) env() Env { return Env{Dir: f.dir, Forge: f.forge, State: f.state} }

// writes counts the forge calls that change something.
func (f *fixture) writes() int {
	n := 0
	for _, c := range f.calls {
		if strings.Contains(c, "query=mutation") {
			n++
		}
	}
	return n
}

// TestReply covers posting a file's exact text on one thread, and the refusals that come before any write.
func TestReply(t *testing.T) {
	f := setup(t)
	f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	f.threads[1] = 2
	text := "Fixed in `abc123`: the \"quoted\" path,\nand a second line.\n"
	body := filepath.Join(t.TempDir(), "reply.md")
	if err := os.WriteFile(body, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Reply(f.env(), 1, "T1", body)
	if err != nil {
		t.Fatal(err)
	}
	if r := f.replies["T1"]; len(r) != 1 || r[0] != text {
		t.Fatalf("posted %q, want %q", r, text)
	}
	if got.ID != "T1" || got.Comments != 2 || got.Last != text {
		t.Errorf("read back %+v", got)
	}

	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, []byte(" \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"foreign thread": func() error { _, err := Reply(f.env(), 1, "T9", body); return err },
		"missing file":   func() error { _, err := Reply(f.env(), 1, "T0", filepath.Join(t.TempDir(), "none")); return err },
		"empty file":     func() error { _, err := Reply(f.env(), 1, "T0", empty); return err },
	} {
		before := f.writes()
		if err := call(); err == nil || f.writes() != before {
			t.Errorf("%s: err %v, %d writes", name, err, f.writes()-before)
		}
	}
}

// TestResolve covers resolving named threads, the all-or-nothing refusal of a foreign id, an already resolved thread, and that status stops listing a resolved one.
func TestResolve(t *testing.T) {
	f := setup(t)
	f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	f.threads[1] = 3

	if _, err := Resolve(f.env(), 1, []string{"T0", "T9"}); err == nil || !strings.Contains(err.Error(), "T9") || f.writes() != 0 {
		t.Fatalf("a foreign id: err %v, %d writes", err, f.writes())
	}

	states, err := Resolve(f.env(), 1, []string{"T0", "T2"})
	if err != nil || len(states) != 2 || !states[0].Resolved || !states[1].Resolved || f.writes() != 2 {
		t.Fatalf("resolve: %+v, %v, %d writes", states, err, f.writes())
	}
	if s := f.status(false, 1)[0]; len(s.Threads) != 1 || s.Threads[0].ID != "T1" {
		t.Errorf("status still lists %+v", s.Threads)
	}

	if states, err := Resolve(f.env(), 1, []string{"T0"}); err != nil || !states[0].Resolved || f.writes() != 2 {
		t.Errorf("resolving a resolved thread: %+v, %v, %d writes", states, err, f.writes())
	}
}
