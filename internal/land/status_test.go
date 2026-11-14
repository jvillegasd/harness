package land

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func (f *fixture) status(stack bool, numbers ...int) []Status {
	f.t.Helper()
	statuses, err := Statuses(Env{Dir: f.dir, Forge: f.forge, State: f.state}, numbers, stack)
	if err != nil {
		f.t.Fatal(err)
	}
	return statuses
}

// TestStatusBlockers covers the verdict and the exit code that names the first blocker, in the order address-review works them.
func TestStatusBlockers(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	f.checks[1] = `[{"name":"ci","bucket":"pass"}]`

	if s := f.status(false, 1)[0]; !s.Ready || len(s.Holds) != 0 || s.Code() != ExitReady || len(s.Threads) != 0 {
		t.Fatalf("a passing pull request got %+v", s)
	}

	for name, tc := range map[string]struct {
		change func()
		code   int
		hold   string
	}{
		"failing check": {func() { f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"fail"}]`, errors.New("exit status 1") }, ExitFailing, "required check ci is fail"},
		"pending check": {func() {
			f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"pending"}]`, errors.New("exit status 8")
		}, ExitPending, "required check ci is pending"},
		"thread": {func() { f.threads[1] = 1 }, ExitThread, "1 review thread"},
		"conflict and failing check": {func() {
			p.Mergeable = "CONFLICTING"
			f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"fail"}]`, errors.New("exit status 1")
		}, ExitConflict, "CONFLICTING"},
		"draft":    {func() { p.IsDraft = true }, ExitOther, "draft"},
		"approval": {func() { p.ReviewDecision = "REVIEW_REQUIRED" }, ExitOther, "approval is missing"},
	} {
		t.Run(name, func(t *testing.T) {
			saved, checks := *p, f.checks[1]
			defer func() { *p, f.checks[1], f.threads[1] = saved, checks, 0; delete(f.checksErr, 1) }()
			tc.change()
			s := f.status(false, 1)[0]
			if s.Ready || s.Code() != tc.code || Code([]Status{s}) != tc.code {
				t.Fatalf("got ready %v, code %d, want %d: %q", s.Ready, s.Code(), tc.code, s.Holds)
			}
			if !slicesContain(s.Holds, tc.hold) {
				t.Errorf("holds %q do not name %q", s.Holds, tc.hold)
			}
		})
	}
}

func slicesContain(holds []string, want string) bool {
	for _, h := range holds {
		if strings.Contains(h, want) {
			return true
		}
	}
	return false
}

// TestStatusThreads covers the unresolved threads the verdict lists, each located and attributed, and that resolved ones are left out.
func TestStatusThreads(t *testing.T) {
	f := setup(t)
	f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	f.threads[1], f.resolved[1] = 2, 1

	s := f.status(false, 1)[0]
	if s.Code() != ExitThread || len(s.Threads) != 2 {
		t.Fatalf("code %d, threads %+v", s.Code(), s.Threads)
	}
	want := Thread{ID: "T0", Path: "a.go", Line: 3, Author: "rev", Permission: "write", Body: `fix "this"`, URL: "https://x/0"}
	if s.Threads[0] != want {
		t.Errorf("thread %+v, want %+v", s.Threads[0], want)
	}
	// One permission read per author.
	asked := 0
	for _, c := range f.calls {
		if strings.Contains(c, "/collaborators/") {
			asked++
		}
	}
	if asked != 1 {
		t.Errorf("read the author's permission %d times", asked)
	}
}

// TestStatusMatchesLandCheck covers that pr status and land check --merge can never disagree about the same head.
func TestStatusMatchesLandCheck(t *testing.T) {
	f := setup(t)
	f.open(1, "feature", "main", map[string]string{"a.go": "changed\n"})
	f.threads[1] = 1
	f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"pending"}]`, errors.New("exit status 8")
	reviewed := f.git(f.dir, "rev-parse", "HEAD")
	f.pulls[1].Body = "Reviewed: " + reviewed + ".." + reviewed

	c, s := f.one(1), f.status(false, 1)[0]
	if !reflect.DeepEqual(c.Review, s.Review) || !reflect.DeepEqual(c.Holds, s.Holds) || !reflect.DeepEqual(c.Findings, s.Findings) {
		t.Errorf("land check %+v %q, pr status %+v %q", c.Review, c.Holds, s.Review, s.Holds)
	}
	if s.Review == nil || s.Review.Status != "current" {
		t.Errorf("review %+v", s.Review)
	}
}

// TestStatusStack covers --stack reporting every open pull request under the tip, bottom-up.
func TestStatusStack(t *testing.T) {
	f := setup(t)
	f.open(1, "one", "main", map[string]string{"README.md": "one\n"})
	f.open(2, "two", "one", map[string]string{"README.md": "two\n"})
	f.open(3, "three", "two", map[string]string{"README.md": "three\n"})

	statuses := f.status(true, 3)
	var got []int
	for _, s := range statuses {
		got = append(got, s.Number)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("stack order %v", got)
	}
}

// TestStatusWritesNothing covers that a status pass only reads the forge and creates no local branch.
func TestStatusWritesNothing(t *testing.T) {
	f := setup(t)
	f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	f.threads[1] = 1
	f.git(f.dir, "switch", "-q", "main")
	before := f.git(f.dir, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/tags")

	f.status(false, 1)
	if after := f.git(f.dir, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/tags"); after != before {
		t.Errorf("local refs went from %q to %q", before, after)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "--method") || strings.Contains(c, "-X ") || strings.Contains(c, "mutation") ||
			strings.HasPrefix(c, "pr edit") || strings.HasPrefix(c, "pr merge") || strings.HasPrefix(c, "pr comment") || strings.HasPrefix(c, "pr review") {
			t.Errorf("a status pass wrote to the forge: %s", c)
		}
	}
}
