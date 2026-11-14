package reviewledger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Forge runs one gh command and returns its stdout.
type Forge func(args ...string) ([]byte, error)

// Gh is the Forge that runs the gh CLI in dir.
func Gh(dir string) Forge {
	return func(args ...string) ([]byte, error) {
		cmd := exec.Command("gh", args...)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			return stdout.Bytes(), fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return stdout.Bytes(), nil
	}
}

// Target is what a review reads: the change base..sha as full commit ids, the ledger key it records under, whether it is foreign (anything but the current task branch, so the review is report-only), and whether it is a fork's pull request (reviewed without running anything).
type Target struct {
	Kind    string `json:"kind"` // branch, pr, or range; the current task branch is a branch
	Base    string `json:"base"`
	SHA     string `json:"sha"`
	Key     string `json:"key"`
	Foreign bool   `json:"foreign"`
	Fork    bool   `json:"fork"`
}

var (
	prURLRe = regexp.MustCompile(`^https?://[^/]+/[^/]+/[^/]+/pull/([0-9]+)(?:[/?#].*)?$`)
	prNumRe = regexp.MustCompile(`^#?([0-9]+)$`)
)

// ResolveTarget resolves a review target: none for the current task branch, a pull request number or URL, a local or origin/ branch, or a <base>..<sha> range. It fetches what it reads from origin without tags and creates no branch.
func ResolveTarget(root string, forge Forge, arg string) (Target, error) {
	current, _ := git(root, "branch", "--show-current")
	if arg == "" {
		if current == "" {
			return Target{}, fmt.Errorf("%w: the head is detached; name a target", ErrInvalid)
		}
		arg = current
	}
	if m := prURLRe.FindStringSubmatch(arg); m != nil {
		return pullTarget(root, forge, m[1], current)
	}
	if m := prNumRe.FindStringSubmatch(arg); m != nil {
		return pullTarget(root, forge, m[1], current)
	}
	if from, to, ok := strings.Cut(arg, ".."); ok && !strings.HasPrefix(to, ".") {
		base, err := commit(root, from)
		if err != nil {
			return Target{}, err
		}
		sha, err := commit(root, to)
		if err != nil {
			return Target{}, err
		}
		return Target{Kind: "range", Base: base, SHA: sha, Key: "range-" + base[:12] + "-" + sha[:12], Foreign: true}, nil
	}
	return branchTarget(root, forge, arg, current)
}

func commit(root, rev string) (string, error) {
	full, err := git(root, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%w: %q does not name a commit", ErrInvalid, rev)
	}
	return full, nil
}

func pullTarget(root string, forge Forge, number, current string) (Target, error) {
	raw, err := forge("pr", "view", number, "--json", "headRefName,headRefOid,baseRefName,isCrossRepository")
	if err != nil {
		return Target{}, err
	}
	var p struct {
		HeadRefName, HeadRefOid, BaseRefName string
		IsCrossRepository                    bool
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.HeadRefOid == "" {
		return Target{}, fmt.Errorf("gh pr view %s: unreadable output", number)
	}
	if _, err := git(root, "fetch", "--quiet", "--no-tags", "origin", p.BaseRefName); err != nil {
		return Target{}, err
	}
	if _, err := git(root, "fetch", "--quiet", "--no-tags", "origin", "refs/pull/"+number+"/head"); err != nil {
		return Target{}, err
	}
	fetched, err := git(root, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return Target{}, err
	}
	if fetched != p.HeadRefOid {
		return Target{}, fmt.Errorf("the fetched head %s is not the head the forge reports, %s; run again", fetched, p.HeadRefOid)
	}
	base, err := git(root, "merge-base", "refs/remotes/origin/"+p.BaseRefName, p.HeadRefOid)
	if err != nil {
		return Target{}, err
	}
	t := Target{Kind: "pr", Base: base, SHA: p.HeadRefOid, Key: p.HeadRefName, Foreign: true, Fork: p.IsCrossRepository}
	if t.Fork {
		t.Key = "pr-" + number
	} else if head, _ := git(root, "rev-parse", "HEAD"); p.HeadRefName == current && head == p.HeadRefOid {
		// The pull request of the branch checked out here, at its head, is the current task branch.
		t.Foreign = false
	}
	return t, nil
}

func branchTarget(root string, forge Forge, name, current string) (Target, error) {
	ref, key := "refs/heads/"+name, name
	if _, err := git(root, "rev-parse", "--verify", "--quiet", ref); err != nil {
		remote, ok := strings.CutPrefix(name, "origin/")
		if !ok {
			return Target{}, fmt.Errorf("%w: %q is not a pull request, a local branch, an origin/ branch, or a range", ErrInvalid, name)
		}
		if _, err := git(root, "fetch", "--quiet", "--no-tags", "origin", remote); err != nil {
			return Target{}, fmt.Errorf("%w: origin has no branch %q", ErrInvalid, remote)
		}
		ref, key = "refs/remotes/origin/"+remote, remote
	}
	sha, err := commit(root, ref)
	if err != nil {
		return Target{}, err
	}
	raw, err := forge("pr", "list", "--state", "open", "--head", key, "--json", "baseRefName,isCrossRepository")
	if err != nil {
		return Target{}, err
	}
	var pulls []struct {
		BaseRefName       string
		IsCrossRepository bool
	}
	if err := json.Unmarshal(raw, &pulls); err != nil {
		return Target{}, fmt.Errorf("gh pr list: unreadable output: %w", err)
	}
	into := ""
	for _, p := range pulls {
		if !p.IsCrossRepository {
			into = "refs/remotes/origin/" + p.BaseRefName
			if _, err := git(root, "fetch", "--quiet", "--no-tags", "origin", p.BaseRefName); err != nil {
				return Target{}, err
			}
		}
	}
	if into == "" {
		for _, r := range []string{"refs/remotes/origin/HEAD", "refs/remotes/origin/main", "refs/remotes/origin/master"} {
			if _, err := git(root, "rev-parse", "--verify", "--quiet", r); err == nil {
				into = r
				break
			}
		}
	}
	if into == "" {
		return Target{}, fmt.Errorf("no default branch ref (origin/HEAD, origin/main, origin/master) to take the base from")
	}
	base, err := git(root, "merge-base", into, sha)
	if err != nil {
		return Target{}, err
	}
	return Target{Kind: "branch", Base: base, SHA: sha, Key: key, Foreign: key != current}, nil
}
