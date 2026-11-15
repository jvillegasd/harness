package reviewledger

import (
	"os"
	"strings"
	"testing"
)

func posted(id, sha, disposition, severity, path, summary, evidence string) Entry {
	return Entry{ID: id, Branch: "pr-7", Base: "base000000000000", SHA: sha, Depth: "high", Path: &path, Lines: "4-9", Severity: severity, Summary: summary, Disposition: disposition, Evidence: evidence}
}

// TestCommentBody covers what a foreign review posts: the latest review's range and level and its confirmed findings with their proof, never a dropped finding, a nit, or an earlier review's finding.
func TestCommentBody(t *testing.T) {
	entries := []Entry{
		posted("old", "sha1111111111111", "confirmed", "bug", "a.go", "earlier review", "red"),
		posted("f1", "sha2222222222222", "confirmed", "bug", "b.go", "nil map write", "TestX panics"),
		posted("f2", "sha2222222222222", "dropped", "risk", "c.go", "unproven race", "stayed green"),
		posted("f3", "sha2222222222222", "nit", "nit", "d.go", "naming", ""),
	}
	body, err := CommentBody(entries, "pr-7")
	if err != nil {
		t.Fatal(err)
	}
	want := "Review of `base00000000..sha222222222` at level high.\n\nConfirmed findings, each proven by a run:\n\n- **bug** `b.go:4-9`: nil map write\n  Proved by: TestX panics\n"
	if body != want {
		t.Fatalf("body\n%q\nwant\n%q", body, want)
	}

	clean := append(entries[:1:1], Entry{ID: "c", Branch: "pr-7", Base: "base000000000000", SHA: "sha3333333333333", Depth: "medium", Disposition: "clean"})
	if body, err := CommentBody(clean, "pr-7"); err != nil || !strings.HasSuffix(body, "No finding was confirmed.\n") || !strings.Contains(body, "sha333333333` at level medium") {
		t.Errorf("a clean review got %q, %v", body, err)
	}
	if _, err := CommentBody(entries, "pr-8"); err == nil {
		t.Error("a key with no review made a comment")
	}
}

// TestPostComment covers the one forge write: the comment goes from a file, as one gh pr comment.
func TestPostComment(t *testing.T) {
	var calls [][]string
	var sent string
	forge := func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		raw, _ := os.ReadFile(args[len(args)-1])
		sent = string(raw)
		return nil, nil
	}
	if err := PostComment(forge, 7, "text `x`\n"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || strings.Join(calls[0][:4], " ") != "pr comment 7 --body-file" || sent != "text `x`\n" {
		t.Errorf("calls %q sent %q", calls, sent)
	}
}

// A later disposition on an older review's finding keeps its reviewed commit, so it must not make that older review the one posted.
func TestCommentBodyIgnoresLaterUpdates(t *testing.T) {
	old := posted("old", "sha1111111111111", "confirmed", "bug", "a.go", "old bug", "red")
	entries := []Entry{
		old,
		posted("new", "sha2222222222222", "confirmed", "bug", "b.go", "new bug", "red2"),
	}
	update := old
	update.Disposition, update.Evidence = "accepted", "human: intended"
	entries = append(entries, update)
	body, err := CommentBody(entries, "pr-7")
	if err != nil || !strings.Contains(body, "sha222222222") || !strings.Contains(body, "new bug") || strings.Contains(body, "old bug") {
		t.Errorf("got %q, %v", body, err)
	}
}
