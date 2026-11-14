package land

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/reviewledger"
)

// fixture is a checkout cloned from a bare origin, and a forge that answers from the pull requests the test registers.
type fixture struct {
	t         *testing.T
	dir       string
	state     string
	pulls     map[int]*pull
	checks    map[int]string // `gh pr checks --json` output; absent means no required checks
	checksErr map[int]error
	threads   map[int]int    // unresolved review threads
	reviews   map[int]string // pages of the reviews endpoint
}

func setup(t *testing.T) *fixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	base := t.TempDir()
	f := &fixture{t: t, dir: filepath.Join(base, "work"), state: filepath.Join(home, "state.json"), pulls: map[int]*pull{}, checks: map[int]string{}, checksErr: map[int]error{}, threads: map[int]int{}, reviews: map[int]string{}}
	f.git(base, "init", "-q", "--bare", "-b", "main", "origin.git")
	f.git(base, "clone", "-q", "origin.git", "work")
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	f.commit(map[string]string{"a.go": strings.Join(lines, "\n") + "\n", "README.md": "readme\n"})
	f.git(f.dir, "push", "-q", "-u", "origin", "main")
	return f
}

func (f *fixture) git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit writes files in the checkout, commits them on the current branch, and returns the commit.
func (f *fixture) commit(files map[string]string) string {
	f.t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(f.dir, name), []byte(body), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	f.git(f.dir, "add", "-A")
	f.git(f.dir, "commit", "-q", "-m", "change")
	return f.git(f.dir, "rev-parse", "HEAD")
}

// open creates branch from base with files, pushes it, and registers it as a mergeable open pull request by the forge user.
func (f *fixture) open(number int, branch, base string, files map[string]string) *pull {
	f.t.Helper()
	f.git(f.dir, "switch", "-q", "-c", branch, base)
	f.commit(files)
	p := &pull{Number: number, State: "OPEN", HeadRefName: branch, BaseRefName: base, Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"}
	p.Author.Login = "me"
	f.pulls[number] = p
	f.push(p)
	return p
}

// push publishes the checkout's head as the pull request's head.
func (f *fixture) push(p *pull) {
	f.t.Helper()
	p.HeadRefOid = f.git(f.dir, "rev-parse", "HEAD")
	f.git(f.dir, "push", "-q", "--force", "origin", "HEAD:refs/heads/"+p.HeadRefName, fmt.Sprintf("HEAD:refs/pull/%d/head", p.Number))
}

func (f *fixture) forge(args ...string) ([]byte, error) {
	arg := func(flag string) string {
		for i, a := range args {
			if a == flag && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	list := func(keep func(*pull) bool) ([]byte, error) {
		out := []*pull{}
		for n := 1; n <= len(f.pulls)+1; n++ {
			if p := f.pulls[n]; p != nil && p.State == "OPEN" && keep(p) {
				out = append(out, p)
			}
		}
		return json.Marshal(out)
	}
	switch args[0] + " " + args[1] {
	case "repo view":
		return []byte(`{"defaultBranchRef":{"name":"main"}}`), nil
	case "api user":
		return []byte("me\n"), nil
	case "pr view":
		number, _ := strconv.Atoi(args[2])
		for _, p := range f.pulls {
			if p.Number == number || number == 0 && p.HeadRefName == f.git(f.dir, "branch", "--show-current") {
				return json.Marshal(p)
			}
		}
		return nil, errors.New("no pull requests found")
	case "pr checks":
		number, _ := strconv.Atoi(args[2])
		if raw, ok := f.checks[number]; ok || f.checksErr[number] != nil {
			return []byte(raw), f.checksErr[number]
		}
		return nil, errors.New("no required checks reported on the branch")
	case "api graphql":
		number, _ := strconv.Atoi(strings.TrimPrefix(args[len(args)-1], "number="))
		nodes := `{"isResolved":true}` + strings.Repeat(`,{"isResolved":false}`, f.threads[number])
		return []byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[` + nodes + `]}}}}}`), nil
	case "pr list":
		if base := arg("--base"); base != "" {
			return list(func(p *pull) bool { return p.BaseRefName == base })
		}
		return list(func(p *pull) bool { return p.HeadRefName == arg("--head") })
	case "api --paginate":
		number, _ := strconv.Atoi(strings.Split(args[2], "/")[4])
		if raw, ok := f.reviews[number]; ok {
			return []byte(raw), nil
		}
		return []byte("[]"), nil
	}
	return nil, fmt.Errorf("unexpected gh %v", args)
}

func (f *fixture) run(merge, stack bool, numbers ...int) []Check {
	f.t.Helper()
	checks, err := Run(Env{Dir: f.dir, Forge: f.forge, State: f.state}, numbers, stack, merge)
	if err != nil {
		f.t.Fatal(err)
	}
	return checks
}

// one checks a single pull request with the merge requested.
func (f *fixture) one(number int) Check {
	f.t.Helper()
	return f.run(true, false, number)[0]
}

func (f *fixture) level(level string) {
	f.t.Helper()
	if err := os.WriteFile(f.state, []byte(`{"review":{"level":"`+level+`"}}`), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// holds asserts the verdict holds for exactly the reasons containing each want, in order.
func holds(t *testing.T, c Check, want ...string) {
	t.Helper()
	if len(c.Holds) != len(want) {
		t.Fatalf("holds %q, want %d containing %q", c.Holds, len(want), want)
	}
	for i, w := range want {
		if !strings.Contains(c.Holds[i], w) {
			t.Errorf("hold %q does not mention %q", c.Holds[i], w)
		}
	}
}

// TestLane covers which pull request states merge, clean up, or stop, and that only an explicit merge request opens the merge lane.
func TestLane(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})

	if c := f.run(false, false)[0]; c.Number != 1 || c.Lane != "stop" {
		t.Fatalf("an open pull request with no merge request got %+v", c)
	} else {
		holds(t, c, "still open")
	}
	if c := f.one(1); c.Lane != "open" || c.Head != p.HeadRefOid {
		t.Fatalf("a merge request got lane %q head %q", c.Lane, c.Head)
	} else {
		holds(t, c)
	}
	p.IsDraft = true
	holds(t, f.one(1), "draft")
	p.IsDraft, p.State = false, "CLOSED"
	holds(t, f.one(1), "CLOSED without a merge")

	// A merged pull request is cleaned up with or without the merge request, and the forge's merge gates no longer apply to it.
	p.State, p.Mergeable, p.MergeStateStatus = "MERGED", "UNKNOWN", "UNKNOWN"
	f.threads[1] = 2
	if c := f.run(false, false, 1)[0]; c.Lane != "merged" {
		t.Fatalf("a merged pull request got lane %q", c.Lane)
	} else {
		holds(t, c)
	}
}

// TestGates covers each forge condition that holds an open pull request.
func TestGates(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	f.checks[1] = `[{"name":"ci","bucket":"pass"},{"name":"optional","bucket":"skipping"}]`
	holds(t, f.one(1))

	for name, tc := range map[string]struct {
		change func()
		want   string
	}{
		"conflict":      {func() { p.Mergeable = "CONFLICTING" }, "CONFLICTING"},
		"blocked":       {func() { p.MergeStateStatus = "BLOCKED" }, "merge state is BLOCKED"},
		"not computed":  {func() { p.Mergeable, p.MergeStateStatus = "UNKNOWN", "UNKNOWN" }, "still computing"},
		"approval":      {func() { p.ReviewDecision = "REVIEW_REQUIRED" }, "approval is missing"},
		"changes":       {func() { p.ReviewDecision = "CHANGES_REQUESTED" }, "requested changes"},
		"thread":        {func() { f.threads[1] = 1 }, "1 review thread"},
		"stacked base":  {func() { p.BaseRefName = "other" }, "not the default branch"},
		"failing check": {func() { f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"fail"}]`, errors.New("exit status 1") }, "required check ci is fail"},
		"pending check": {func() {
			f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"pending"}]`, errors.New("exit status 8")
		}, "required check ci is pending"},
		"checks unusable": {func() { f.checks[1], f.checksErr[1] = "", errors.New("HTTP 502") }, "required checks are unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			saved, checks := *p, f.checks[1]
			defer func() { *p, f.checks[1], f.threads[1] = saved, checks, 0; delete(f.checksErr, 1) }()
			tc.change()
			if tc.want == "not the default branch" {
				f.git(f.dir, "push", "-q", "origin", "main:refs/heads/other")
			}
			holds(t, f.one(1), tc.want)
		})
	}

	// A branch with no required checks is not a hold.
	delete(f.checks, 1)
	holds(t, f.one(1))
}

// TestMode covers which checkout a landing may touch: this one only when it is the pull request's head.
func TestMode(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	if c := f.one(1); c.Mode != "local" || c.Key != "docs" || len(c.Worktrees) != 0 {
		t.Fatalf("the checkout at the head got %+v", c)
	}

	// Ahead of the forge's head, this checkout holds work the merge would not carry.
	f.commit(map[string]string{"README.md": "unpushed\n"})
	if c := f.one(1); c.Mode != "remote" {
		t.Errorf("a checkout ahead of the head got mode %q", c.Mode)
	} else {
		holds(t, c, "push or reset it")
	}
	p.State = "MERGED"
	if c := f.one(1); c.Mode != "remote" || len(c.Notes) == 0 || !strings.Contains(strings.Join(c.Notes, "\n"), "left alone") {
		t.Errorf("a merged pull request whose checkout moved on got mode %q notes %q", c.Mode, c.Notes)
	}
	// Behind the merged head, it is still that pull request's checkout.
	f.git(f.dir, "reset", "-q", "--hard", "HEAD~2")
	if c := f.one(1); c.Mode != "local" {
		t.Errorf("a checkout behind the merged head got mode %q", c.Mode)
	}
	p.State = "OPEN"
	f.git(f.dir, "reset", "-q", "--hard", p.HeadRefOid)

	other := filepath.Join(filepath.Dir(f.dir), "other")
	f.git(f.dir, "switch", "-q", "main")
	f.git(f.dir, "worktree", "add", "-q", other, "docs")
	if c := f.one(1); c.Mode != "remote" || len(c.Worktrees) != 1 || filepath.Base(c.Worktrees[0]) != "other" {
		t.Errorf("a pull request held by another worktree got mode %q worktrees %q", c.Mode, c.Worktrees)
	}

	// A fork's branch name can match a local one without being it.
	p.IsCrossRepository = true
	if c := f.one(1); c.Mode != "fork" || c.Key != "pr-1" || len(c.Worktrees) != 0 || len(c.Dependents) != 0 {
		t.Errorf("a fork got %+v", c)
	}
}

// TestReviewRequirement covers when a review is required: by level, by the changed files' tier, and by the tier the body claims.
func TestReviewRequirement(t *testing.T) {
	f := setup(t)
	docs := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	code := f.open(2, "code", "main", map[string]string{"b.go": "package b\n"})

	if r := f.one(1).Review; r.Required || r.Tier != "tests-docs" || r.Level != "medium" {
		t.Errorf("a documentation change got %+v", r)
	}
	docs.Body = "## Review guide\n\nTiers: critical 0, behavioral 1, structural 0, tests-docs 1, mechanical 0\n"
	if r := f.one(1).Review; !r.Required || r.Tier != "behavioral" {
		t.Errorf("a body raising the tier got %+v", r)
	}

	c := f.one(2)
	if r := c.Review; !r.Required || r.Tier != "structural" || r.Source != "none" || r.Status != "missing" {
		t.Errorf("an unreviewed code change got %+v", r)
	}
	holds(t, c, "required review never ran")
	// A body cannot lower the floor the files set.
	code.Body = "Tiers: critical 0, behavioral 0, structural 0, tests-docs 1, mechanical 0\n"
	holds(t, f.one(2), "required review never ran")

	f.level("off")
	if c := f.one(2); c.Review.Required || len(c.Holds) != 0 {
		t.Errorf("level off still required a review: %+v holds %q", c.Review, c.Holds)
	}
	f.level("medium")

	// Merged with a merge commit, the head is on the base, and the change is still what that merge brought in.
	f.git(f.dir, "switch", "-q", "main")
	f.git(f.dir, "merge", "-q", "--no-ff", "-m", "merge", "code")
	f.git(f.dir, "push", "-q", "origin", "main")
	code.State, code.MergeCommit.Oid = "MERGED", f.git(f.dir, "rev-parse", "HEAD")
	c = f.one(2)
	if c.Review.Tier != "structural" {
		t.Errorf("a pull request merged with a merge commit got %+v", c.Review)
	}
	holds(t, c, "required review never ran")
}

// TestReviewCoverage covers whether the reviewed commit still stands for the head: unchanged, moved by a rebase, followed by tests, followed by code, or gone.
func TestReviewCoverage(t *testing.T) {
	f := setup(t)
	edited := strings.Replace(f.read("a.go"), "line 18", "line eighteen", 1)
	p := f.open(1, "code", "main", map[string]string{"a.go": edited, "a_test.go": "package a\n"})
	reviewed := p.HeadRefOid
	p.Body = "Body.\n\nReviewed: " + f.git(f.dir, "rev-parse", "main") + ".." + reviewed + "\n"

	c := f.one(1)
	if r := c.Review; r.Source != "body" || r.SHA != reviewed || r.Status != "current" {
		t.Fatalf("a head at the reviewed commit got %+v", r)
	}
	holds(t, c)

	// The default branch moves under the same file and the branch is rebased: the patch is the one that was reviewed.
	f.git(f.dir, "switch", "-q", "main")
	f.commit(map[string]string{"a.go": "added above\n" + f.read("a.go"), "c.go": "package c\n"})
	f.git(f.dir, "push", "-q", "origin", "main")
	f.git(f.dir, "switch", "-q", "code")
	f.git(f.dir, "rebase", "-q", "main")
	f.push(p)
	if r := f.one(1).Review; r.Status != "current" || len(r.Changed) != 0 {
		t.Fatalf("a rebased, unchanged patch got %+v", r)
	}

	f.commit(map[string]string{"a_test.go": "package a\n\n// more\n"})
	f.push(p)
	c = f.one(1)
	if r := c.Review; r.Status != "tests-docs" || strings.Join(r.Changed, ",") != "a_test.go" {
		t.Fatalf("a later test edit got %+v", r)
	}
	holds(t, c, "Read their hunks")

	f.commit(map[string]string{"a.go": strings.Replace(f.read("a.go"), "line 3", "line three", 1)})
	f.push(p)
	c = f.one(1)
	if r := c.Review; r.Status != "stale" || strings.Join(r.Changed, ",") != "a.go,a_test.go" {
		t.Fatalf("a later code edit got %+v", r)
	}
	holds(t, c, "is stale: a.go, a_test.go")

	// A stale review that was never required holds nothing.
	f.level("off")
	if c := f.one(1); c.Review.Status != "stale" || len(c.Holds) != 0 || len(c.Notes) == 0 {
		t.Errorf("at level off a stale review got holds %q notes %q", c.Holds, c.Notes)
	}
	f.level("medium")

	p.Body = "Reviewed: 1111111111111111111111111111111111111111..2222222222222222222222222222222222222222\n"
	c = f.one(1)
	if c.Review.Status != "stale" {
		t.Errorf("a reviewed commit that is gone got %+v", c.Review)
	}
	holds(t, c, "reviewed commit is gone")
}

// TestReviewSources covers the two sources beside the body: the ledger's clean line, and a person's approval, which only someone else's pull request may rest on.
func TestReviewSources(t *testing.T) {
	f := setup(t)
	p := f.open(1, "code", "main", map[string]string{"b.go": "package b\n"})
	approve := func(reviews string) { f.reviews[1] = strings.ReplaceAll(reviews, "HEAD", p.HeadRefOid) }
	const approved = `[{"state":"APPROVED","commit_id":"HEAD","user":{"login":"ana","type":"User"}}]`

	approve(approved)
	if r := f.one(1).Review; r.Source != "none" {
		t.Errorf("the forge user's own pull request took an approval as its review: %+v", r)
	}

	p.Author.Login = "coworker"
	c := f.one(1)
	if r := c.Review; c.Mine || r.Source != "approval" || r.Status != "current" {
		t.Fatalf("an approved pull request of someone else's got mine %v review %+v", c.Mine, r)
	}
	holds(t, c)
	for name, reviews := range map[string]string{
		"a bot's approval":              `[{"state":"APPROVED","commit_id":"HEAD","user":{"login":"bot","type":"Bot"}}]`,
		"a comment":                     `[{"state":"COMMENTED","commit_id":"HEAD","user":{"login":"ana","type":"User"}}]`,
		"another person's standing ask": approved + `[{"state":"CHANGES_REQUESTED","commit_id":"HEAD","user":{"login":"bo","type":"User"}}]`,
	} {
		approve(reviews)
		if r := f.one(1).Review; r.Source != "none" {
			t.Errorf("%s counted as a review: %+v", name, r)
		}
	}
	// A reviewer's later approval replaces their own change request, across pages.
	approve(`[{"state":"CHANGES_REQUESTED","commit_id":"HEAD","user":{"login":"ana","type":"User"}}]` + approved)
	if r := f.one(1).Review; r.Source != "approval" {
		t.Errorf("a change request its author later approved got %+v", r)
	}

	// The ledger's clean line is read before any approval, for anyone's pull request.
	file, err := reviewledger.File(f.dir, "code")
	if err != nil {
		t.Fatal(err)
	}
	clean := reviewledger.Entry{ID: "c1", Branch: "code", Base: "main", SHA: p.HeadRefOid, Depth: "medium", Disposition: "clean"}
	if err := reviewledger.Append(file, clean); err != nil {
		t.Fatal(err)
	}
	delete(f.reviews, 1)
	if r := f.one(1).Review; r.Source != "ledger" || r.Status != "current" {
		t.Errorf("a clean ledger line got %+v", r)
	}
}

// TestFindings covers which ledger findings hold a merge: an open one and a recorded dismissal, never a dropped or a fixed one.
func TestFindings(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	if c := f.one(1); len(c.Notes) != 1 || !strings.Contains(c.Notes[0], "no review ledger") {
		t.Errorf("an empty ledger got notes %q", c.Notes)
	}
	reviewed := f.git(f.dir, "rev-parse", "HEAD~1")
	file, err := reviewledger.File(f.dir, "docs")
	if err != nil {
		t.Fatal(err)
	}
	path := "README.md"
	for id, e := range map[string]reviewledger.Entry{
		"open":      {Disposition: "confirmed"},
		"dismissed": {Disposition: "accepted", Evidence: "intended"},
		"dropped":   {Disposition: "dropped", Evidence: "stayed green"},
		"fixed":     {Disposition: "fixed", Evidence: "green", Commit: p.HeadRefOid},
	} {
		e.ID, e.Branch, e.SHA, e.Depth, e.Path, e.Severity, e.Summary = id, "docs", reviewed, "medium", &path, "bug", id+" summary"
		if err := reviewledger.Append(file, e); err != nil {
			t.Fatal(err)
		}
	}
	c := f.one(1)
	if c.Dropped != 1 || len(c.Findings) != 2 || len(c.Holds) != 2 || len(c.Notes) != 0 {
		t.Fatalf("got dropped %d findings %+v holds %q notes %q", c.Dropped, c.Findings, c.Holds, c.Notes)
	}
	all := strings.Join(c.Holds, "\n")
	for _, want := range []string{"finding open is open: README.md open summary", "finding dismissed was dismissed"} {
		if !strings.Contains(all, want) {
			t.Errorf("holds %q lack %q", c.Holds, want)
		}
	}
}

// TestRun covers the order pull requests land in, the stack walk, and whose dependents a landing may repair.
func TestRun(t *testing.T) {
	f := setup(t)
	f.open(1, "one", "main", map[string]string{"README.md": "one\n"})
	f.open(2, "two", "one", map[string]string{"README.md": "two\n"})
	theirs := f.open(3, "three", "one", map[string]string{"README.md": "three\n"})
	theirs.Author.Login = "coworker"

	// Alone, a pull request stacked on an open base cannot merge.
	holds(t, f.one(2), "not the default branch")

	for name, run := range map[string][]Check{"numbers": f.run(true, false, 2, 1, 2), "stack": f.run(true, true, 2)} {
		if len(run) != 2 || run[0].Number != 1 || run[1].Number != 2 || run[0].After != 0 || run[1].After != 1 {
			t.Fatalf("%s: got order and bases %+v", name, run)
		}
		holds(t, run[1])
		deps := run[0].Dependents
		if len(deps) != 2 || deps[0].Number != 2 || !deps[0].Mine || deps[1].Number != 3 || deps[1].Mine || deps[1].HeadRefOid != theirs.HeadRefOid {
			t.Errorf("%s: dependents %+v", name, deps)
		}
	}

	f.git(f.dir, "branch", "orphan", "main")
	f.git(f.dir, "push", "-q", "origin", "orphan")
	f.open(4, "four", "orphan", map[string]string{"README.md": "four\n"})
	if _, err := Run(Env{Dir: f.dir, Forge: f.forge, State: f.state}, []int{4}, true, true); err == nil || !strings.Contains(err.Error(), "has no open pull request") {
		t.Errorf("a stack on a base with no pull request returned %v", err)
	}
}

func (f *fixture) read(name string) string {
	f.t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(raw)
}
