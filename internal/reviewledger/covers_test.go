package reviewledger

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// coverRepo is a throwaway repository with a base commit on main and a feature branch "feat" that changes a.go, isolated from the user's Git config.
type coverRepo struct {
	t    *testing.T
	dir  string
	base string
}

func newCoverRepo(t *testing.T) *coverRepo {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	r := &coverRepo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line")
	}
	r.commit(map[string]string{"a.go": strings.Join(lines, "\n") + "\n", "b.go": "b\n"})
	r.base = r.git("rev-parse", "HEAD")
	r.git("switch", "-q", "-c", "feat")
	r.commit(map[string]string{"a.go": "changed\n" + strings.Join(lines[1:], "\n") + "\n"})
	return r
}

func (r *coverRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *coverRepo) commit(files map[string]string) string {
	r.t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(body), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "change")
	return r.git("rev-parse", "HEAD")
}

func (r *coverRepo) record(e Entry) {
	r.t.Helper()
	e.Branch = "feat"
	if e.ID == "" {
		e.ID = e.Disposition + e.SHA[:7] + e.Depth
	}
	file, err := File(r.dir, "feat")
	if err != nil {
		r.t.Fatal(err)
	}
	if err := Append(file, e); err != nil {
		r.t.Fatal(err)
	}
}

func (r *coverRepo) clean(sha, depth string, digest *string) {
	r.record(Entry{SHA: sha, Base: r.base, Depth: depth, Disposition: "clean", Digest: digest})
}

func (r *coverRepo) covers(base, sha, level, digest string) Coverage {
	r.t.Helper()
	c, err := Covers(r.dir, "feat", base, sha, level, digest)
	if err != nil {
		r.t.Fatal(err)
	}
	return c
}

// TestCoversDepth covers a clean record covering its own range at its level and below, never above.
func TestCoversDepth(t *testing.T) {
	r := newCoverRepo(t)
	head := r.git("rev-parse", "HEAD")
	if c := r.covers(r.base, head, "medium", ""); c.Covered {
		t.Fatalf("an empty ledger covered: %+v", c)
	}
	r.clean(head, "medium", nil)
	if c := r.covers(r.base, head, "medium", ""); !c.Covered || c.Depth != "medium" || c.SHA != head || len(c.Open) != 0 {
		t.Errorf("medium request: %+v", c)
	}
	if c := r.covers(r.base, head, "high", ""); c.Covered {
		t.Errorf("a medium review covered a high request")
	}
	r.clean(head, "high", nil)
	if c := r.covers(r.base, head, "medium", ""); !c.Covered || c.Depth != "high" {
		t.Errorf("a high review did not cover a medium request: %+v", c)
	}
}

// TestCoversRebase covers a rebase onto a moved base keeping the review, and a later patch change ending it.
func TestCoversRebase(t *testing.T) {
	r := newCoverRepo(t)
	reviewed := r.git("rev-parse", "HEAD")
	r.clean(reviewed, "medium", nil)

	r.git("switch", "-q", "main")
	moved := r.commit(map[string]string{"b.go": "moved\n"})
	r.git("switch", "-q", "feat")
	r.git("rebase", "-q", "main")
	rebased := r.git("rev-parse", "HEAD")
	if c := r.covers(moved, rebased, "medium", ""); !c.Covered || c.SHA != reviewed {
		t.Errorf("a rebase that changed no patch lost its review: %+v", c)
	}

	later := r.commit(map[string]string{"a.go": "changed again\n"})
	if c := r.covers(moved, later, "medium", ""); c.Covered {
		t.Errorf("a later patch change stayed covered: %+v", c)
	}
}

// TestCoversIncomplete covers a review that recorded findings and no clean line, whatever their dispositions.
func TestCoversIncomplete(t *testing.T) {
	r := newCoverRepo(t)
	head := r.git("rev-parse", "HEAD")
	path := "a.go"
	r.record(Entry{SHA: head, Depth: "high", Path: &path, Severity: "nit", Summary: "s", Disposition: "nit"})
	r.record(Entry{ID: "d", SHA: head, Depth: "high", Path: &path, Severity: "bug", Summary: "s", Disposition: "dropped", Evidence: "ran"})
	if c := r.covers(r.base, head, "medium", ""); c.Covered {
		t.Errorf("findings without a clean line covered: %+v", c)
	}
}

// TestCoversDigest covers a digest record covering only the same digest, and committed work only a record without one.
func TestCoversDigest(t *testing.T) {
	r := newCoverRepo(t)
	head := r.git("rev-parse", "HEAD")
	d := "abc123"
	r.clean(head, "medium", &d)
	if c := r.covers(r.base, head, "medium", d); !c.Covered {
		t.Errorf("the same digest was not covered")
	}
	if c := r.covers(r.base, head, "medium", "other"); c.Covered {
		t.Errorf("another digest was covered")
	}
	if c := r.covers(r.base, head, "medium", ""); c.Covered {
		t.Errorf("committed work was covered by a digest review")
	}
}

// TestCoversOpenFindings covers a covered answer listing the findings still open at the asked commit.
func TestCoversOpenFindings(t *testing.T) {
	r := newCoverRepo(t)
	head := r.git("rev-parse", "HEAD")
	path := "a.go"
	r.record(Entry{ID: "open", SHA: head, Depth: "medium", Path: &path, Severity: "bug", Summary: "still", Disposition: "confirmed"})
	r.record(Entry{ID: "gone", SHA: head, Depth: "medium", Path: &path, Severity: "risk", Summary: "s", Disposition: "dropped", Evidence: "ran"})
	r.clean(head, "medium", nil)
	c := r.covers(r.base, head, "medium", "")
	if !c.Covered || len(c.Open) != 1 || c.Open[0].ID != "open" {
		t.Errorf("open findings %+v", c.Open)
	}
}

// TestCoversWritesNothing covers that answering leaves the ledger byte for byte.
func TestCoversWritesNothing(t *testing.T) {
	r := newCoverRepo(t)
	head := r.git("rev-parse", "HEAD")
	r.clean(head, "medium", nil)
	file, _ := File(r.dir, "feat")
	before, _ := os.ReadFile(file)
	r.covers(r.base, head, "high", "")
	r.covers(r.base, head, "medium", "")
	if after, _ := os.ReadFile(file); string(after) != string(before) {
		t.Error("the ledger changed")
	}
}
