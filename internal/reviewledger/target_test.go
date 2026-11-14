package reviewledger

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// targetRepo is a coverRepo cloned from a bare origin: main and feat are pushed, and feat is published as pull request 5.
type targetRepo struct {
	*coverRepo
	head   string // feat's tip
	pulls  map[string]string
	forged string // a head the forge reports instead of the pushed one
}

func newTargetRepo(t *testing.T) *targetRepo {
	t.Helper()
	r := &targetRepo{coverRepo: newCoverRepo(t), pulls: map[string]string{}}
	origin := filepath.Join(t.TempDir(), "origin.git")
	r.git("init", "-q", "--bare", "-b", "main", origin)
	r.git("remote", "add", "origin", origin)
	r.head = r.git("rev-parse", "feat")
	r.git("push", "-q", "origin", "main", "feat", "feat:refs/pull/5/head", "feat:refs/pull/6/head")
	r.git("fetch", "-q", "origin")
	r.git("remote", "set-head", "origin", "main")
	r.pulls["5"] = `{"headRefName":"feat","headRefOid":"%s","baseRefName":"main","isCrossRepository":false}`
	r.pulls["6"] = `{"headRefName":"feat","headRefOid":"%s","baseRefName":"main","isCrossRepository":true}`
	return r
}

func (r *targetRepo) forge(args ...string) ([]byte, error) {
	switch args[0] + " " + args[1] {
	case "pr view":
		head := r.head
		if r.forged != "" {
			head = r.forged
		}
		if p, ok := r.pulls[args[2]]; ok {
			return []byte(fmt.Sprintf(p, head)), nil
		}
		return nil, errors.New("no pull requests found")
	case "pr list":
		if args[5] == "feat" {
			return []byte(`[{"baseRefName":"main","isCrossRepository":false}]`), nil
		}
		return []byte(`[]`), nil
	}
	return nil, fmt.Errorf("unexpected gh %v", args)
}

func (r *targetRepo) resolve(arg string) (Target, error) {
	return ResolveTarget(r.dir, r.forge, arg)
}

func (r *targetRepo) refs() string {
	return r.git("for-each-ref", "--format=%(refname)", "refs/heads", "refs/tags")
}

// TestTargetKinds covers each kind of target resolving to its base, commit, key, and foreignness.
func TestTargetKinds(t *testing.T) {
	r := newTargetRepo(t)
	before := r.refs()
	for name, tc := range map[string]struct {
		arg  string
		want Target
	}{
		"current branch":   {"", Target{Kind: "branch", Base: r.base, SHA: r.head, Key: "feat"}},
		"pull request":     {"5", Target{Kind: "pr", Base: r.base, SHA: r.head, Key: "feat"}},
		"pull request url": {"https://github.com/o/r/pull/5", Target{Kind: "pr", Base: r.base, SHA: r.head, Key: "feat"}},
		"fork":             {"6", Target{Kind: "pr", Base: r.base, SHA: r.head, Key: "pr-6", Foreign: true, Fork: true}},
		"local branch":     {"main", Target{Kind: "branch", Base: r.base, SHA: r.base, Key: "main", Foreign: true}},
		"remote branch":    {"origin/feat", Target{Kind: "branch", Base: r.base, SHA: r.head, Key: "feat"}},
		"range":            {r.base + ".." + r.head, Target{Kind: "range", Base: r.base, SHA: r.head, Key: "range-" + r.base[:12] + "-" + r.head[:12], Foreign: true}},
		"short range":      {"main..feat", Target{Kind: "range", Base: r.base, SHA: r.head, Key: "range-" + r.base[:12] + "-" + r.head[:12], Foreign: true}},
	} {
		got, err := r.resolve(tc.arg)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %+v, %v; want %+v", name, got, err, tc.want)
		}
	}
	// The pull request of another branch is foreign.
	r.git("switch", "-q", "main")
	if got, _ := r.resolve("5"); !got.Foreign {
		t.Errorf("a pull request of another branch was not foreign")
	}
	r.git("switch", "-q", "feat")
	if after := r.refs(); after != before {
		t.Errorf("refs went from %q to %q", before, after)
	}
}

// TestTargetRefusals covers a forge head the fetch does not match, an unknown branch, an unresolvable range end, and a detached head with no target.
func TestTargetRefusals(t *testing.T) {
	r := newTargetRepo(t)
	r.forged = strings.Repeat("a", 40)
	if _, err := r.resolve("5"); err == nil || !strings.Contains(err.Error(), r.forged) || !strings.Contains(err.Error(), r.head) {
		t.Errorf("a mismatched head: %v", err)
	}
	r.forged = ""
	for name, arg := range map[string]string{"unknown branch": "nope", "unresolvable range": "main..nope"} {
		if _, err := r.resolve(arg); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	r.git("switch", "-q", "--detach", "HEAD")
	if _, err := r.resolve(""); !errors.Is(err, ErrInvalid) {
		t.Errorf("a detached head: %v", err)
	}
}
