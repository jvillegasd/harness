package reviewledger

import (
	"errors"
	"strings"
	"testing"
)

func (r *coverRepo) stamp(head, compareBase string) (string, error) {
	r.t.Helper()
	return Stamp(r.dir, "feat", head, compareBase, "high")
}

// TestStampReviewed covers the Reviewed line for a clean record at the head, kept across a rebase that changes no patch and lost to a later patch change.
func TestStampReviewed(t *testing.T) {
	r := newCoverRepo(t)
	reviewed := r.git("rev-parse", "HEAD")
	r.clean(reviewed, "medium", nil)
	if line, err := r.stamp("HEAD", "main"); err != nil || line != "Reviewed: "+r.base+".."+reviewed {
		t.Fatalf("at the head: %q %v", line, err)
	}

	r.git("switch", "-q", "main")
	r.commit(map[string]string{"b.go": "moved\n"})
	r.git("switch", "-q", "feat")
	r.git("rebase", "-q", "main")
	if line, err := r.stamp("HEAD", "main"); err != nil || line != "Reviewed: "+r.base+".."+reviewed {
		t.Errorf("after a rebase: %q %v", line, err)
	}

	r.commit(map[string]string{"a.go": "changed again\n"})
	if line, err := r.stamp("HEAD", "main"); !errors.Is(err, ErrNoStamp) || line != "" || !strings.Contains(err.Error(), "a.go") {
		t.Errorf("after a patch change: %q %v", line, err)
	}
}

// TestStampWithheld covers each ledger that supports no Reviewed line: an open finding, findings with no clean record, and nothing recorded.
func TestStampWithheld(t *testing.T) {
	r := newCoverRepo(t)
	head := r.git("rev-parse", "HEAD")
	if _, err := r.stamp("HEAD", "main"); !errors.Is(err, ErrNoStamp) || !strings.Contains(err.Error(), "no review is recorded") {
		t.Errorf("nothing recorded: %v", err)
	}
	path := "a.go"
	r.record(Entry{ID: "nit", SHA: head, Depth: "medium", Path: &path, Severity: "nit", Summary: "s", Disposition: "nit"})
	if _, err := r.stamp("HEAD", "main"); !errors.Is(err, ErrNoStamp) || !strings.Contains(err.Error(), "not complete") {
		t.Errorf("findings without a clean record: %v", err)
	}
	r.record(Entry{ID: "bug1", SHA: head, Depth: "medium", Path: &path, Severity: "bug", Summary: "breaks", Disposition: "confirmed"})
	r.clean(head, "medium", nil)
	if line, err := r.stamp("HEAD", "main"); !errors.Is(err, ErrNoStamp) || line != "" || !strings.Contains(err.Error(), "bug1") {
		t.Errorf("an open finding: %q %v", line, err)
	}
}

// TestStampNotRun covers the line for a review the level kept from running.
func TestStampNotRun(t *testing.T) {
	r := newCoverRepo(t)
	r.record(Entry{SHA: r.git("rev-parse", "HEAD"), Depth: "none", Disposition: "no-review", Summary: "level medium: tests-docs only"})
	if line, err := r.stamp("HEAD", "main"); err != nil || line != "Review: not run (level medium)" {
		t.Errorf("%q %v", line, err)
	}
}
