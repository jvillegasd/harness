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

	"github.com/jvillegasd/harness/internal/review"
)

type fixture struct {
	t         *testing.T
	dir       string
	state     string
	pulls     map[int]*pull
	checks    map[int]string
	checksErr map[int]error
	threads   map[int]int
	resolved  map[int]int
	comments  map[int]string
	perms     map[string]string
	calls     []string

	resolvedIDs map[string]bool
	replies     map[string][]string

	mergeRefused bool
	mergeBodies  []string

	readBackFails bool
}

func setup(t *testing.T) *fixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	base := t.TempDir()
	f := &fixture{t: t, dir: filepath.Join(base, "work"), state: filepath.Join(home, "state.json"), pulls: map[int]*pull{}, checks: map[int]string{}, checksErr: map[int]error{}, threads: map[int]int{}, resolved: map[int]int{}, comments: map[int]string{}, perms: map[string]string{}, resolvedIDs: map[string]bool{}, replies: map[string][]string{}}
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

func (f *fixture) push(p *pull) {
	f.t.Helper()
	p.HeadRefOid = f.git(f.dir, "rev-parse", "HEAD")
	f.git(f.dir, "push", "-q", "--force", "origin", "HEAD:refs/heads/"+p.HeadRefName, fmt.Sprintf("HEAD:refs/pull/%d/head", p.Number))
}

func (f *fixture) forge(args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
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
		if f.readBackFails && strings.Contains(strings.Join(args, " "), "state,mergeCommit") {
			return nil, errors.New("HTTP 502")
		}
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
		if q := args[3]; strings.HasPrefix(q, "query=mutation") {
			thread := strings.TrimPrefix(args[5], "thread=")
			if strings.Contains(q, "resolveReviewThread") {
				f.resolvedIDs[thread] = true
			} else {
				f.replies[thread] = append(f.replies[thread], strings.TrimPrefix(args[7], "body="))
			}
			return []byte(`{"data":{}}`), nil
		}
		number, _ := strconv.Atoi(strings.TrimPrefix(args[len(args)-1], "number="))

		nodes := make([]string, 0, 1+f.resolved[number]+f.threads[number])
		for i := 0; i < 1+f.resolved[number]; i++ {
			nodes = append(nodes, fmt.Sprintf(`{"id":"R%d","isResolved":true,"comments":{"totalCount":1,"nodes":[]}}`, i))
		}
		for i := 0; i < f.threads[number]; i++ {
			id := fmt.Sprintf("T%d", i)
			comments, _ := json.Marshal(append([]string{`fix "this"`}, f.replies[id]...))
			var bodies []string
			_ = json.Unmarshal(comments, &bodies)
			last, _ := json.Marshal(bodies[len(bodies)-1])

			first := `fix \"this\"`
			if strings.Contains(args[3], "comments(last:1)") {
				first = string(last[1 : len(last)-1])
			}
			nodes = append(nodes, fmt.Sprintf(`{"id":"%s","isResolved":%t,"path":"a.go","line":%d,"comments":{"totalCount":%d,"nodes":[{"author":{"login":"rev"},"body":"%s","url":"https://x/%d"}]}}`, id, f.resolvedIDs[id], i+3, len(bodies), first, i))
		}
		more := len(nodes) > 100
		if more {
			nodes = nodes[:100]
		}
		return []byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":` + strconv.FormatBool(more) + `},"nodes":[` + strings.Join(nodes, ",") + `]}}}}}`), nil
	case "pr list":
		if base := arg("--base"); base != "" {
			return list(func(p *pull) bool { return p.BaseRefName == base })
		}
		return list(func(p *pull) bool { return p.HeadRefName == arg("--head") })
	case "pr merge":
		number, _ := strconv.Atoi(args[2])
		body, _ := os.ReadFile(arg("--body-file"))
		f.mergeBodies = append(f.mergeBodies, string(body))
		if f.mergeRefused {
			return nil, errors.New("merge refused")
		}
		f.pulls[number].State, f.pulls[number].MergeCommit.Oid = "MERGED", "abc123"
		return nil, nil
	case "pr edit":
		number, _ := strconv.Atoi(args[2])
		f.pulls[number].BaseRefName = arg("--base")
		return nil, nil
	case "api --paginate":
		number, _ := strconv.Atoi(strings.Split(args[2], "/")[4])
		if raw, ok := f.comments[number]; ok {
			return []byte(raw), nil
		}
		return []byte("[]"), nil
	}
	if len(args) > 1 && args[0] == "api" && strings.HasSuffix(args[1], "/permission") {
		login := strings.Split(args[1], "/")[4]
		if p, ok := f.perms[login]; ok {
			return []byte(p + "\n"), nil
		}
		return []byte("write\n"), nil
	}
	return nil, fmt.Errorf("unexpected gh %v", args)
}

func (f *fixture) verdict(number int, result, sha, login string) {
	f.t.Helper()
	v := review.Verdict{Result: result, Base: f.git(f.dir, "rev-parse", "origin/main"), SHA: sha, Level: "medium"}
	page, err := json.Marshal([]map[string]any{{
		"body":     review.Body(v, "notes"),
		"html_url": fmt.Sprintf("https://x/%d#%s-%d", number, login, len(f.comments[number])),
		"user":     map[string]string{"login": login},
	}})
	if err != nil {
		f.t.Fatal(err)
	}
	f.comments[number] += string(page)
}

func (f *fixture) runVerdict(number int, result, sha, login string) {
	f.t.Helper()
	page, err := json.Marshal([]map[string]any{{
		"body":     review.Body(review.Verdict{Run: true, Result: result, SHA: sha}, "notes"),
		"html_url": fmt.Sprintf("https://x/%d#%s-run-%d", number, login, len(f.comments[number])),
		"user":     map[string]string{"login": login},
	}})
	if err != nil {
		f.t.Fatal(err)
	}
	f.comments[number] += string(page)
}

func (f *fixture) run(merge, stack bool, numbers ...int) []Check {
	f.t.Helper()
	checks, err := Run(Env{Dir: f.dir, Forge: f.forge, State: f.state}, numbers, stack, merge)
	if err != nil {
		f.t.Fatal(err)
	}
	return checks
}

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

	p.State, p.Mergeable, p.MergeStateStatus = "MERGED", "UNKNOWN", "UNKNOWN"
	f.threads[1] = 2
	if c := f.run(false, false, 1)[0]; c.Lane != "merged" {
		t.Fatalf("a merged pull request got lane %q", c.Lane)
	} else {
		holds(t, c)
	}
}

func TestGates(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	f.checks[1] = `[{"name":"ci","bucket":"pass"},{"name":"optional","bucket":"skipping"}]`
	holds(t, f.one(1))

	for name, tc := range map[string]struct {
		change func()
		want   string
	}{
		"conflict":                   {func() { p.Mergeable = "CONFLICTING" }, "CONFLICTING"},
		"blocked":                    {func() { p.MergeStateStatus = "BLOCKED" }, "merge state is BLOCKED"},
		"dirty":                      {func() { p.MergeStateStatus = "DIRTY" }, "its merge state is DIRTY"},
		"not computed":               {func() { p.Mergeable, p.MergeStateStatus = "UNKNOWN", "UNKNOWN" }, "still computing"},
		"approval":                   {func() { p.ReviewDecision = "REVIEW_REQUIRED" }, "approval is missing"},
		"changes":                    {func() { p.ReviewDecision = "CHANGES_REQUESTED" }, "requested changes"},
		"thread":                     {func() { f.threads[1] = 1 }, "1 review thread"},
		"thread past the first page": {func() { f.resolved[1], f.threads[1] = 100, 1 }, "more than 100 review threads"},
		"stacked base":               {func() { p.BaseRefName = "other" }, "not the default branch"},
		"failing check":              {func() { f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"fail"}]`, errors.New("exit status 1") }, "required check ci is fail"},
		"pending check": {func() {
			f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"pending"}]`, errors.New("exit status 8")
		}, "required check ci is pending"},
		"checks unusable": {func() { f.checks[1], f.checksErr[1] = "", errors.New("HTTP 502") }, "required checks are unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			saved, checks := *p, f.checks[1]
			defer func() { *p, f.checks[1], f.threads[1], f.resolved[1] = saved, checks, 0, 0; delete(f.checksErr, 1) }()
			tc.change()
			if tc.want == "not the default branch" {
				f.git(f.dir, "push", "-q", "origin", "main:refs/heads/other")
			}
			holds(t, f.one(1), tc.want)
		})
	}

	delete(f.checks, 1)
	holds(t, f.one(1))
}

func TestMode(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	if c := f.one(1); c.Mode != "local" || c.Key != "docs" || len(c.Worktrees) != 0 {
		t.Fatalf("the checkout at the head got %+v", c)
	}

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

	p.IsCrossRepository = true
	if c := f.one(1); c.Mode != "fork" || c.Key != "pr-1" || len(c.Worktrees) != 0 || len(c.Dependents) != 0 {
		t.Errorf("a fork got %+v", c)
	}
}

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

	code.Body = "Tiers: critical 0, behavioral 0, structural 0, tests-docs 1, mechanical 0\n"
	holds(t, f.one(2), "required review never ran")

	f.level("off")
	if c := f.one(2); c.Review.Required || len(c.Holds) != 0 {
		t.Errorf("level off still required a review: %+v holds %q", c.Review, c.Holds)
	}
	f.level("medium")

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

func TestReviewCoverage(t *testing.T) {
	f := setup(t)
	edited := strings.Replace(f.read("a.go"), "line 18", "line eighteen", 1)
	p := f.open(1, "code", "main", map[string]string{"a.go": edited, "a_test.go": "package a\n"})
	reviewed := p.HeadRefOid
	f.verdict(1, "PASS", reviewed, "me")

	c := f.one(1)
	if r := c.Review; r.Source != "verdict" || r.Result != "PASS" || r.SHA != reviewed || r.Status != "current" {
		t.Fatalf("a head at the reviewed commit got %+v", r)
	}
	holds(t, c)

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

	f.level("off")
	if c := f.one(1); c.Review.Status != "stale" || len(c.Holds) != 0 || len(c.Notes) == 0 {
		t.Errorf("at level off a stale review got holds %q notes %q", c.Holds, c.Notes)
	}
	f.level("medium")

	f.verdict(1, "PASS", strings.Repeat("2", 40), "me")
	c = f.one(1)
	if c.Review.Status != "stale" {
		t.Errorf("a reviewed commit that is gone got %+v", c.Review)
	}
	holds(t, c, "reviewed commit is gone")
}

func TestVerdictSources(t *testing.T) {
	f := setup(t)
	p := f.open(1, "code", "main", map[string]string{"b.go": "package b\n"})
	p.Author.Login = "coworker"
	p.Body = "Reviewed: " + f.git(f.dir, "rev-parse", "origin/main") + ".." + p.HeadRefOid + "\n"
	f.comments[1] = `[{"body":"LGTM","html_url":"https://x/1#c","user":{"login":"ana"}}]`
	if r := f.one(1).Review; r.Source != "none" || r.Status != "missing" {
		t.Errorf("a body line and a plain comment counted as a review: %+v", r)
	}

	f.perms["drive-by"] = "read"
	f.verdict(1, "PASS", p.HeadRefOid, "drive-by")
	if r := f.one(1).Review; r.Source != "none" || len(r.Ignored) != 1 || !strings.Contains(r.Ignored[0], "drive-by (permission read)") {
		t.Errorf("a reader's verdict counted, or was not reported ignored: %+v", r)
	}

	f.verdict(1, "FAIL", p.HeadRefOid, "ana")
	f.verdict(1, "PASS", p.HeadRefOid, "bo")
	c := f.one(1)
	if r := c.Review; r.Source != "verdict" || r.Result != "PASS" || r.Status != "current" || !strings.Contains(r.URL, "#bo-") {
		t.Fatalf("the newest writer's verdict did not win: %+v", r)
	}
	holds(t, c)
}

func TestVerdictBase(t *testing.T) {
	f := setup(t)
	p := f.open(1, "code", "main", map[string]string{"b.go": "package b\n"})
	first := p.HeadRefOid
	f.commit(map[string]string{"c.go": "package c\n"})
	f.push(p)
	v := review.Verdict{Result: "PASS", Base: first, SHA: p.HeadRefOid, Level: "medium"}
	page, err := json.Marshal([]map[string]any{{"body": review.Body(v, ""), "html_url": "https://x/1#partial", "user": map[string]string{"login": "me"}}})
	if err != nil {
		t.Fatal(err)
	}
	f.comments[1] = string(page)
	c := f.one(1)
	if r := c.Review; r.Status != "stale" {
		t.Fatalf("a verdict over part of the change got %+v", r)
	}
	holds(t, c, "after the pull request's base")
}

func TestVerdictResults(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	failed := p.HeadRefOid
	f.verdict(1, "FAIL", failed, "me")
	holds(t, f.one(1), "is FAIL")

	f.git(f.dir, "switch", "-q", "docs")
	f.commit(map[string]string{"README.md": "fixed\n"})
	f.push(p)
	holds(t, f.one(1), "is FAIL")

	f.verdict(1, "PASS+NOTES", p.HeadRefOid, "me")
	c := f.one(1)
	holds(t, c)
	if r := c.Review; r.Result != "PASS+NOTES" || r.Status != "current" || len(c.Notes) != 1 || !strings.Contains(c.Notes[0], "read its notes") {
		t.Errorf("a PASS+NOTES verdict got review %+v notes %q", r, c.Notes)
	}
}

func TestRunVerdict(t *testing.T) {
	f := setup(t)
	p := f.open(1, "code", "main", map[string]string{"b.go": "package b\n", "b_test.go": "package b\n"})
	f.verdict(1, "PASS", p.HeadRefOid, "me")
	c := f.one(1)
	if r := c.RunVerdict; r.Required || r.Source != "none" || r.Status != "missing" {
		t.Fatalf("a structural change got run %+v", r)
	}
	holds(t, c)

	p.Body = "Tiers: critical 0, behavioral 1, structural 0, tests-docs 1, mechanical 0\n"
	c = f.one(1)
	if r := c.RunVerdict; !r.Required || r.Source != "none" {
		t.Fatalf("a behavioral change with only a review verdict got run %+v", r)
	}
	holds(t, c, "run-as-user verdict was never posted")
	f.level("off")
	holds(t, f.one(1), "run-as-user verdict was never posted")
	f.level("medium")

	f.perms["drive-by"] = "read"
	f.runVerdict(1, "PASS", p.HeadRefOid, "drive-by")
	holds(t, f.one(1), "run-as-user verdict was never posted")

	driven := p.HeadRefOid
	f.runVerdict(1, "PASS", driven, "me")
	c = f.one(1)
	if r := c.RunVerdict; r.Source != "verdict" || r.Result != "PASS" || r.SHA != driven || r.Status != "current" || c.Review.Result != "PASS" || c.Review.Status != "current" {
		t.Fatalf("a head at the driven commit got run %+v review %+v", r, c.Review)
	}
	holds(t, c)

	f.git(f.dir, "switch", "-q", "code")
	f.commit(map[string]string{"b_test.go": "package b\n\n// more\n"})
	f.push(p)
	c = f.one(1)
	if r := c.RunVerdict; r.Status != "tests-docs" {
		t.Fatalf("a later test edit got run %+v", r)
	}
	holds(t, c, "Read their hunks")

	f.commit(map[string]string{"b.go": "package b\n\nvar B = 1\n"})
	f.push(p)
	f.verdict(1, "PASS", p.HeadRefOid, "me")
	c = f.one(1)
	if r := c.RunVerdict; r.Status != "stale" || strings.Join(r.Changed, ",") != "b.go,b_test.go" {
		t.Fatalf("a later code edit got run %+v", r)
	}
	holds(t, c, "run-as-user verdict of "+driven[:12]+" is stale: b.go, b_test.go")

	f.runVerdict(1, "FAIL", p.HeadRefOid, "me")
	holds(t, f.one(1), "latest run-as-user verdict, of "+p.HeadRefOid[:12]+", is FAIL")
	f.runVerdict(1, "PASS+NOTES", p.HeadRefOid, "me")
	c = f.one(1)
	holds(t, c)
	if len(c.Notes) != 1 || !strings.Contains(c.Notes[0], "run-as-user verdict") || !strings.Contains(c.Notes[0], "read its notes") {
		t.Errorf("a PASS+NOTES run verdict got notes %q", c.Notes)
	}

	q := f.open(2, "docs", "main", map[string]string{"README.md": "more\n"})
	f.runVerdict(2, "FAIL", q.HeadRefOid, "me")
	holds(t, f.one(2), "latest run-as-user verdict")
}

func TestRun(t *testing.T) {
	f := setup(t)
	f.open(1, "one", "main", map[string]string{"README.md": "one\n"})
	f.open(2, "two", "one", map[string]string{"README.md": "two\n"})
	theirs := f.open(3, "three", "one", map[string]string{"README.md": "three\n"})
	theirs.Author.Login = "coworker"

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
