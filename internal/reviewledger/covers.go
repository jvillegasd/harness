package reviewledger

import (
	"fmt"
	"slices"
)

// Coverage answers whether a recorded review already covers a patch. When it does, it names the covering review and the findings still open at the asked commit.
type Coverage struct {
	Covered bool      `json:"covered"`
	Depth   string    `json:"depth,omitempty"`
	Base    string    `json:"base,omitempty"`
	SHA     string    `json:"sha,omitempty"`
	Open    []Finding `json:"open"`
}

// deeper reports whether a review of depth reaches level: high covers medium, never the reverse.
func deeper(depth, level string) bool {
	rank := map[string]int{"medium": 1, "high": 2}
	return rank[depth] > 0 && rank[depth] >= rank[level]
}

// Covers reports whether the branch's ledger holds a clean record, at level or deeper, of the same change as base..sha, file by file. A record made with a digest covers only a request with that same digest, and one made without covers only committed work. The comparison is the one `harness land check` makes, so a rebase that changes no file's patch keeps the review.
func Covers(root, branch, base, sha, level, digest string) (Coverage, error) {
	if level != "medium" && level != "high" {
		return Coverage{}, fmt.Errorf("%w: level %q is not medium or high", ErrInvalid, level)
	}
	resolve := func(rev string) (string, error) {
		full, err := git(root, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
		if err != nil {
			return "", fmt.Errorf("%w: %q does not name a commit", ErrInvalid, rev)
		}
		return full, nil
	}
	var err error
	if base, err = resolve(base); err != nil {
		return Coverage{}, err
	}
	if sha, err = resolve(sha); err != nil {
		return Coverage{}, err
	}
	file, err := File(root, branch)
	if err != nil {
		return Coverage{}, err
	}
	entries, err := Read(file)
	if err != nil {
		return Coverage{}, err
	}
	touched, err := Names(root, base+"..."+sha)
	if err != nil {
		return Coverage{}, err
	}

	out := Coverage{Open: []Finding{}}
	// Only the latest qualifying record counts, as in land check: an older review a later one superseded covers nothing.
	for _, e := range slices.Backward(entries) {
		if e.Branch != branch || e.Disposition != "clean" || !deeper(e.Depth, level) {
			continue
		}
		if e.Digest != nil && *e.Digest != digest || e.Digest == nil && digest != "" {
			continue
		}
		// A reviewed commit that is gone cannot be compared, so it covers nothing.
		if reviewed, err := resolve(e.SHA); err == nil {
			changed, err := ChangedSince(root, base, reviewed, sha, touched)
			if err != nil {
				return Coverage{}, err
			}
			if len(changed) == 0 {
				out.Covered, out.Depth, out.Base, out.SHA = true, e.Depth, e.Base, reviewed
			}
		}
		break
	}
	if !out.Covered {
		return out, nil
	}
	for _, f := range Latest(entries, branch, func(c string) bool {
		_, err := git(root, "merge-base", "--is-ancestor", c, sha)
		return err == nil
	}) {
		if f.Open {
			out.Open = append(out.Open, f)
		}
	}
	return out, nil
}
