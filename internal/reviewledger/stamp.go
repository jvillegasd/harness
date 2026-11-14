package reviewledger

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrNoStamp marks a ledger that supports no review line for the head, so a pull request body gets none.
var ErrNoStamp = errors.New("no review line")

var summaryLevelRe = regexp.MustCompile(`\blevel (off|medium|high)\b`)

// Stamp returns the one review line a pull request body carries for head, taken from the branch's ledger: "Reviewed: <base>..<sha>" when the latest clean record still covers head and no finding is open, or "Review: not run (level <level>)" when the latest whole-review record says no review ran. Otherwise it returns ErrNoStamp with the reason. compareBase is what the reviewed change and head's are compared against, as land check compares them against the pull request's base; level is used when a no-review record names none.
func Stamp(root, branch, head, compareBase, level string) (string, error) {
	none := func(format string, a ...any) (string, error) {
		return "", fmt.Errorf("%w: %s", ErrNoStamp, fmt.Sprintf(format, a...))
	}
	head, err := git(root, "rev-parse", "--verify", "--quiet", "--end-of-options", head+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%w: the head does not name a commit", ErrInvalid)
	}
	file, err := File(root, branch)
	if err != nil {
		return "", err
	}
	entries, err := Read(file)
	if err != nil {
		return "", err
	}
	var whole, clean *Entry
	findings := 0
	for i := range entries {
		e := &entries[i]
		switch {
		case e.Branch != branch:
		case e.Disposition == "no-review":
			whole = e
		case e.Disposition == "clean":
			whole = e
			// A digest review read uncommitted work, which no pull request head holds.
			if e.Digest == nil {
				clean = e
			}
		default:
			findings++
		}
	}
	if whole != nil && whole.Disposition == "no-review" {
		if m := summaryLevelRe.FindStringSubmatch(whole.Summary); m != nil {
			level = m[1]
		}
		return fmt.Sprintf("Review: not run (level %s)", level), nil
	}

	var open []string
	for _, f := range Latest(entries, branch, func(c string) bool {
		_, err := git(root, "merge-base", "--is-ancestor", c, head)
		return err == nil
	}) {
		if f.Open {
			path := ""
			if f.Path != nil {
				path = " " + *f.Path
			}
			open = append(open, fmt.Sprintf("%s (%s%s: %s)", f.ID, f.Disposition, path, f.Summary))
		}
	}
	switch {
	case len(open) > 0:
		return none("a finding is still open: %s", strings.Join(open, "; "))
	case clean == nil && findings > 0:
		return none("the review is not complete: findings are recorded and no clean record, which a review that left a part unreviewed never writes")
	case clean == nil:
		return none("no review is recorded for %s", branch)
	case clean.Base == "":
		return none("the clean record %s names no base", clean.ID)
	}
	base, err := git(root, "rev-parse", "--verify", "--quiet", "--end-of-options", clean.Base+"^{commit}")
	if err != nil {
		return none("the clean record's base %s is gone", clean.Base)
	}
	reviewed, err := git(root, "rev-parse", "--verify", "--quiet", "--end-of-options", clean.SHA+"^{commit}")
	if err != nil {
		return none("the reviewed commit %s is gone", clean.SHA)
	}
	if compareBase == "" {
		compareBase = base
	}
	touched, err := Names(root, compareBase+"..."+head)
	if err != nil {
		return "", err
	}
	changed, err := ChangedSince(root, compareBase, reviewed, head, touched)
	if err != nil {
		return "", err
	}
	if len(changed) > 0 {
		return none("the review never read the change to %s", strings.Join(changed, ", "))
	}
	return fmt.Sprintf("Reviewed: %s..%s", base, reviewed), nil
}
