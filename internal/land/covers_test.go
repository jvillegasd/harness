package land

import (
	"testing"

	"github.com/jvillegasd/harness/internal/reviewledger"
)

// TestCoversMatchesLandCheck covers that for a pull request whose only review source is the ledger, review covers answers covered exactly when land check reports the review current.
func TestCoversMatchesLandCheck(t *testing.T) {
	f := setup(t)
	p := f.open(1, "feature", "main", map[string]string{"a.go": "changed\n"})
	file, err := reviewledger.File(f.dir, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewledger.Append(file, reviewledger.Entry{ID: "c", Branch: "feature", SHA: p.HeadRefOid, Depth: "medium", Disposition: "clean"}); err != nil {
		t.Fatal(err)
	}

	for _, step := range []struct {
		name   string
		change map[string]string
	}{
		{"at the reviewed head", nil},
		{"after a docs edit", map[string]string{"README.md": "more docs\n"}},
		{"after a code edit", map[string]string{"a.go": "changed again\n"}},
	} {
		if step.change != nil {
			f.git(f.dir, "switch", "-q", "feature")
			f.commit(step.change)
			f.push(p)
		}
		c := f.one(1)
		cov, err := reviewledger.Covers(f.dir, "feature", "origin/main", p.HeadRefOid, "medium", "")
		if err != nil {
			t.Fatal(err)
		}
		if c.Review.Source != "ledger" || cov.Covered != (c.Review.Status == "current") {
			t.Errorf("%s: land check %s %s, covers %v", step.name, c.Review.Source, c.Review.Status, cov.Covered)
		}
	}
}

// TestStampRoundTrips covers that the line review stamp prints reads back through land check's parser as a current body review, before and after a rebase that changes no patch.
func TestStampRoundTrips(t *testing.T) {
	f := setup(t)
	base := f.git(f.dir, "rev-parse", "main")
	p := f.open(1, "feature", "main", map[string]string{"a.go": "changed\n"})
	file, err := reviewledger.File(f.dir, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewledger.Append(file, reviewledger.Entry{ID: "c", Branch: "feature", Base: base, SHA: p.HeadRefOid, Depth: "medium", Disposition: "clean"}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"at the reviewed head", "after a rebase"} {
		if step == "after a rebase" {
			f.advance()
			f.git(f.dir, "rebase", "-q", "origin/main")
			f.push(p)
		}
		line, err := reviewledger.Stamp(f.dir, "feature", p.HeadRefOid, "origin/main", "medium")
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		p.Body = "Some description.\n\n" + line + "\n"
		if r := f.one(1).Review; r.Source != "body" || r.Status != "current" {
			t.Errorf("%s: %q reads as %s %s", step, line, r.Source, r.Status)
		}
	}
}
