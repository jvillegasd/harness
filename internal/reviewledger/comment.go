package reviewledger

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CommentBody is the one comment a foreign review posts, built from the ledger's latest review of key: the reviewed range and level, and each finding confirmed in it with the run that proved it. Dropped findings, nits, and hunter reasoning never reach it. It is an error when the ledger holds no review for key.
func CommentBody(entries []Entry, key string) (string, error) {
	latest := Latest(entries, key, func(string) bool { return true })
	// The latest review is the one whose line was written last; its findings share its reviewed commit.
	var last *Entry
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Branch == key && entries[i].Disposition != "no-review" {
			last = &entries[i]
			break
		}
	}
	if last == nil {
		return "", fmt.Errorf("%w: the ledger holds no review of %s", ErrInvalid, key)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Review of `%s..%s` at level %s.\n\n", short(last.Base), short(last.SHA), last.Depth)
	var confirmed []Finding
	for _, f := range latest {
		if f.SHA == last.SHA && f.Base == last.Base && f.Disposition == "confirmed" && f.Path != nil {
			confirmed = append(confirmed, f)
		}
	}
	if len(confirmed) == 0 {
		b.WriteString("No finding was confirmed.\n")
		return b.String(), nil
	}
	b.WriteString("Confirmed findings, each proven by a run:\n")
	for _, f := range confirmed {
		where := *f.Path
		if f.Lines != "" {
			where += ":" + f.Lines
		}
		fmt.Fprintf(&b, "\n- **%s** `%s`: %s\n", f.Severity, where, f.Summary)
		if f.Evidence != "" {
			fmt.Fprintf(&b, "  Proved by: %s\n", f.Evidence)
		}
	}
	return b.String(), nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// PostComment posts body on pull request number as one comment through forge, from a file so the text never passes through a shell.
func PostComment(forge Forge, number int, body string) error {
	tmp, err := os.MkdirTemp("", "harness-review-comment-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "comment.md")
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		return err
	}
	_, err = forge("pr", "comment", strconv.Itoa(number), "--body-file", file)
	return err
}
