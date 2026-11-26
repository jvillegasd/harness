package land

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jvillegasd/harness/internal/run"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jvillegasd/harness/internal/changemap"
	"github.com/jvillegasd/harness/internal/review"
	"github.com/jvillegasd/harness/internal/worktree"
)

type Env struct {
	Dir   string
	Forge run.Forge
	State string
}

type Check struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Lane        string `json:"lane"`
	Mode        string `json:"mode,omitempty"`
	Mine        bool   `json:"mine"`
	Head        string `json:"head"`
	HeadRefName string `json:"headRefName"`
	Base        string `json:"base"`
	Default     string `json:"default"`

	After int `json:"after,omitempty"`

	Key string `json:"key"`

	Worktrees  []string    `json:"worktrees"`
	Review     *Review     `json:"review,omitempty"`
	RunVerdict *RunVerdict `json:"run,omitempty"`
	Dependents []Dependent `json:"dependents"`
	Holds      []string    `json:"holds"`
	Notes      []string    `json:"notes"`

	gate gateFacts

	body string
}

type gateFacts struct {
	conflict, failing, pending bool

	computing bool
	threads   []Thread

	holds []string
}

type Thread struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Author string `json:"author"`

	Permission string `json:"permission"`
	Body       string `json:"body"`
	URL        string `json:"url"`
}

type Review struct {
	Required bool   `json:"required"`
	Level    string `json:"level"`
	Tier     string `json:"tier"`
	Source   string `json:"source"`

	Result string `json:"result,omitempty"`
	Depth  string `json:"depth,omitempty"`
	URL    string `json:"url,omitempty"`

	Base   string `json:"base,omitempty"`
	SHA    string `json:"sha,omitempty"`
	Status string `json:"status"`

	Changed []string `json:"changed,omitempty"`

	Ignored []string `json:"ignored,omitempty"`
}

type RunVerdict struct {
	Required bool   `json:"required"`
	Source   string `json:"source"`

	Result string `json:"result,omitempty"`
	URL    string `json:"url,omitempty"`
	SHA    string `json:"sha,omitempty"`
	Status string `json:"status"`

	Changed []string `json:"changed,omitempty"`
}

type Dependent struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`

	Mine bool `json:"mine"`
}

type pull struct {
	Number            int
	URL               string
	Title             string
	State             string
	IsDraft           bool
	HeadRefName       string
	HeadRefOid        string
	BaseRefName       string
	IsCrossRepository bool
	Author            struct{ Login string }
	Mergeable         string
	MergeStateStatus  string
	ReviewDecision    string
	Body              string
	MergeCommit       struct{ Oid string }
}

const viewFields = "number,url,title,state,isDraft,headRefName,headRefOid,baseRefName,isCrossRepository,author,mergeable,mergeStateStatus,reviewDecision,body,mergeCommit"

const threadsQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){pullRequest(number:$number){reviewThreads(first:100){pageInfo{hasNextPage}nodes{id isResolved path line originalLine comments(first:1){nodes{author{login}body url}}}}}}}`

func Run(e Env, numbers []int, stack, merge bool) ([]Check, error) {
	run, err := e.resolve(numbers, stack)
	if err != nil {
		return nil, err
	}
	checks := []Check{}
	for i, p := range run.pulls {
		c, err := e.check(run.root, p, run.def, run.me, merge, run.below(p, run.pulls[:i]))
		if err != nil {
			return nil, fmt.Errorf("#%d: %w", p.Number, err)
		}
		checks = append(checks, c)
	}
	return checks, nil
}

type landing struct {
	root, def, me string
	pulls         []pull
}

func (landing) below(p pull, among []pull) int {
	for _, q := range among {
		if q.Number != p.Number && q.State == "OPEN" && !q.IsCrossRepository && q.HeadRefName == p.BaseRefName {
			return q.Number
		}
	}
	return 0
}

func (e Env) resolve(numbers []int, stack bool) (landing, error) {
	var l landing
	root, err := run.Git(e.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return l, errors.New("not inside a git repository")
	}
	var repo struct{ DefaultBranchRef struct{ Name string } }
	if err := e.read(&repo, "repo", "view", "--json", "defaultBranchRef"); err != nil {
		return l, err
	}
	def := repo.DefaultBranchRef.Name
	raw, err := e.Forge("api", "user", "--jq", ".login")
	me := strings.TrimSpace(string(raw))
	if err != nil || me == "" || def == "" {
		return l, fmt.Errorf("cannot read the forge user and default branch: %v", err)
	}

	var pulls []pull
	if len(numbers) == 0 {
		numbers = []int{0}
	}
	for _, n := range numbers {
		p, err := e.view(n)
		if err != nil {
			return l, err
		}
		if !slices.ContainsFunc(pulls, func(q pull) bool { return q.Number == p.Number }) {
			pulls = append(pulls, p)
		}
	}

	for p := pulls[0]; stack && p.State == "OPEN" && p.BaseRefName != def && len(pulls) < 50; {
		var heads []struct {
			Number            int
			IsCrossRepository bool
		}
		if err := e.read(&heads, "pr", "list", "--state", "open", "--head", p.BaseRefName, "--json", "number,isCrossRepository"); err != nil {
			return l, err
		}
		under := 0
		for _, h := range heads {
			if !h.IsCrossRepository {
				under = h.Number
			}
		}
		if under == 0 {
			return l, fmt.Errorf("#%d is based on %s, which is not the default branch and has no open pull request", p.Number, p.BaseRefName)
		}
		if p, err = e.view(under); err != nil {
			return l, err
		}
		pulls = append(pulls, p)
	}

	var ordered []pull
	for rest := pulls; len(rest) > 0; {
		var blocked []pull
		for _, p := range rest {
			if l.below(p, rest) != 0 {
				blocked = append(blocked, p)
			} else {
				ordered = append(ordered, p)
			}
		}
		if len(blocked) == len(rest) {
			return l, errors.New("the pull requests are based on each other in a cycle")
		}
		rest = blocked
	}

	return landing{root, def, me, ordered}, nil
}

func (e Env) check(root string, p pull, def, me string, merge bool, after int) (Check, error) {
	c := Check{
		Number: p.Number, URL: p.URL, Title: p.Title, Mine: p.Author.Login == me,
		Head: p.HeadRefOid, HeadRefName: p.HeadRefName, Base: p.BaseRefName, Default: def, After: after, Key: p.HeadRefName,
		Worktrees: []string{}, Dependents: []Dependent{}, Holds: []string{}, Notes: []string{},
		body: p.Body,
	}
	hold := func(format string, args ...any) { c.Holds = append(c.Holds, fmt.Sprintf(format, args...)) }
	note := func(format string, args ...any) { c.Notes = append(c.Notes, fmt.Sprintf(format, args...)) }
	if p.IsCrossRepository {
		c.Key = "pr-" + strconv.Itoa(p.Number)
	}
	switch {
	case p.State == "MERGED":
		c.Lane = "merged"
	case p.State != "OPEN":
		c.Lane = "stop"
		hold("it is %s without a merge: keeping or discarding its work is the human's decision", p.State)
	case p.IsDraft:
		c.Lane = "stop"
		hold("it is a draft")
	case !merge:
		c.Lane = "stop"
		hold("it is still open: merge it and run /land again, or run /land merge")
	default:
		c.Lane = "open"
	}
	if c.Lane == "stop" {
		return c, nil
	}
	open := c.Lane == "open"

	if _, err := run.Git(root, "fetch", "--quiet", "origin", fmt.Sprintf("refs/pull/%d/head", p.Number)); err != nil {
		return c, err
	}
	if _, err := run.Git(root, "cat-file", "-e", p.HeadRefOid+"^{commit}"); err != nil {
		return c, fmt.Errorf("the fetched pull request does not hold its head %s; run again", p.HeadRefOid)
	}
	baseRef := "origin/" + p.BaseRefName
	if _, err := run.Git(root, "fetch", "--quiet", "origin", p.BaseRefName); err != nil {
		baseRef = ""
	} else if _, err := run.Git(root, "merge-base", "--is-ancestor", p.HeadRefOid, baseRef); err == nil && p.MergeCommit.Oid != "" {

		baseRef = p.MergeCommit.Oid + "^1"
	}

	trees, err := worktree.List(root)
	if err != nil {
		return c, err
	}
	c.Mode = "remote"
	if p.IsCrossRepository {
		c.Mode = "fork"
	}
	for _, t := range trees {
		switch {

		case p.IsCrossRepository || t.Branch != p.HeadRefName:
		case t.Path != root:
			c.Worktrees = append(c.Worktrees, t.Path)
		case t.Head == p.HeadRefOid:
			c.Mode = "local"
		case open:
			hold("this checkout's %s is at %s but the pull request's head is %s: push or reset it, or land from another checkout", t.Branch, short(t.Head), short(p.HeadRefOid))
		default:

			if _, err := run.Git(root, "merge-base", "--is-ancestor", t.Head, p.HeadRefOid); err == nil {
				c.Mode = "local"
			} else {
				note("this checkout's %s is not this pull request's head (a reused name or unpushed commits), so it is left alone", t.Branch)
			}
		}
	}

	if open {
		before := len(c.Holds)
		e.gates(p, &c, def)
		c.gate.holds = slices.Clone(c.Holds[before:])
	}
	if err := e.review(root, p, &c, baseRef); err != nil {
		return c, err
	}
	if !p.IsCrossRepository {
		var deps []struct {
			Number            int
			HeadRefName       string
			HeadRefOid        string
			IsCrossRepository bool
			Author            struct{ Login string }
		}
		if err := e.read(&deps, "pr", "list", "--state", "open", "--base", p.HeadRefName, "--json", "number,headRefName,headRefOid,author,isCrossRepository"); err != nil {
			return c, err
		}
		for _, d := range deps {
			c.Dependents = append(c.Dependents, Dependent{d.Number, d.HeadRefName, d.HeadRefOid, d.Author.Login == me && !d.IsCrossRepository})
		}
	}
	return c, nil
}

func (e Env) gates(p pull, c *Check, def string) {
	hold := func(format string, args ...any) { c.Holds = append(c.Holds, fmt.Sprintf(format, args...)) }
	if p.BaseRefName != def && c.After == 0 {
		hold("its base %s is not the default branch, so merging would squash it into that base: land the base's pull request first, or retarget it with gh pr edit %d --base %s once that base has merged", p.BaseRefName, p.Number, def)
	}

	switch {
	case p.Mergeable == "UNKNOWN" || p.MergeStateStatus == "UNKNOWN":
		c.gate.computing = true
		hold("the forge is still computing whether it can merge: check again")
	case p.Mergeable != "MERGEABLE":
		c.gate.conflict = p.Mergeable == "CONFLICTING"
		hold("the forge reports it %s, not MERGEABLE", p.Mergeable)
	case !slices.Contains([]string{"CLEAN", "UNSTABLE", "HAS_HOOKS"}, p.MergeStateStatus):
		c.gate.conflict = p.MergeStateStatus == "DIRTY"
		hold("its merge state is %s", p.MergeStateStatus)
	}
	switch p.ReviewDecision {
	case "REVIEW_REQUIRED":
		hold("a required approval is missing")
	case "CHANGES_REQUESTED":
		hold("a reviewer requested changes")
	}

	var checks []struct{ Name, Bucket string }
	raw, err := e.Forge("pr", "checks", strconv.Itoa(p.Number), "--required", "--json", "name,bucket")

	if json.Unmarshal(raw, &checks) != nil && (err == nil || !strings.Contains(err.Error(), "checks reported")) {
		hold("its required checks are unavailable: %v", err)
	}
	for _, check := range checks {
		if check.Bucket != "pass" && check.Bucket != "skipping" {
			hold("required check %s is %s", check.Name, check.Bucket)
		}
		switch check.Bucket {
		case "fail", "cancel":
			c.gate.failing = true
		case "pending":
			c.gate.pending = true
		}
	}

	var threads struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						PageInfo struct{ HasNextPage bool }
						Nodes    []struct {
							ID                 string
							IsResolved         bool
							Path               string
							Line, OriginalLine int
							Comments           struct {
								Nodes []struct {
									Author    struct{ Login string }
									Body, URL string
								}
							}
						}
					}
				}
			}
		}
	}
	if err := e.read(&threads, "api", "graphql", "-f", "query="+threadsQuery, "-F", "owner={owner}", "-F", "name={repo}", "-F", "number="+strconv.Itoa(p.Number)); err != nil {
		hold("its review threads are unavailable: %v", err)
	}
	unresolved := 0
	for _, t := range threads.Data.Repository.PullRequest.ReviewThreads.Nodes {
		if t.IsResolved {
			continue
		}
		unresolved++

		th := Thread{ID: t.ID, Path: t.Path, Line: cmp.Or(t.Line, t.OriginalLine)}
		if first := t.Comments.Nodes; len(first) > 0 {
			th.Author, th.Body, th.URL = first[0].Author.Login, first[0].Body, first[0].URL
		}
		c.gate.threads = append(c.gate.threads, th)
	}
	if unresolved > 0 {
		hold("%d review thread(s) are unresolved", unresolved)
	}
	if threads.Data.Repository.PullRequest.ReviewThreads.PageInfo.HasNextPage {
		hold("it has more than 100 review threads, and those past the first 100 are unread; resolve them on the forge")
	}
}

var (
	tiersRe = regexp.MustCompile(`(?m)^Tiers:.*$`)
	tierRe  = regexp.MustCompile(`(critical|behavioral|structural)\s+[1-9]`)
)

func (e Env) review(root string, p pull, c *Check, baseRef string) error {
	hold := func(format string, args ...any) { c.Holds = append(c.Holds, fmt.Sprintf(format, args...)) }
	note := func(format string, args ...any) { c.Notes = append(c.Notes, fmt.Sprintf(format, args...)) }
	level, _, err := review.LoadLevel(e.State)
	if err != nil {
		return err
	}
	r := &Review{Level: level, Source: "none", Status: "missing"}
	rv := &RunVerdict{Source: "none", Status: "missing"}
	c.Review, c.RunVerdict = r, rv
	head := p.HeadRefOid

	reviewed, drove, ignored, err := e.verdicts(p.Number)
	if err != nil {
		return err
	}
	r.Ignored = ignored
	if v := reviewed; v != nil {
		r.Source, r.Result, r.Depth, r.URL, r.Base, r.SHA = "verdict", v.Result, v.Level, v.url, v.Base, v.SHA
	}
	if v := drove; v != nil {
		rv.Source, rv.Result, rv.URL, rv.SHA = "verdict", v.Result, v.url, v.SHA
	}

	for _, v := range []struct{ kind, result, sha, url string }{{"review", r.Result, r.SHA, r.URL}, {"run-as-user", rv.Result, rv.SHA, rv.URL}} {
		switch v.result {
		case "FAIL":
			hold("the latest %s verdict, of %s, is FAIL: %s", v.kind, short(v.sha), v.url)
		case "PASS+NOTES":
			note("the latest %s verdict, of %s, is PASS+NOTES; read its notes: %s", v.kind, short(v.sha), v.url)
		}
	}

	if baseRef == "" {
		r.Required, r.Status = level != "off", "stale"
		if rv.Source != "none" {
			rv.Status = "stale"
		}
		hold("its base branch %s is gone from origin, so no review can be compared with what merged", p.BaseRefName)
		return nil
	}
	mergeBase, err := run.Git(root, "merge-base", baseRef, head)
	if err != nil {
		return err
	}
	cfg, err := changemap.EffectiveConfig(root, mergeBase)
	if err != nil {
		return err
	}

	if c.Mode == "local" {
		if cfg.Locked, err = changemap.LoadLocks(root); err != nil {
			return err
		}
	}
	touched, err := names(root, baseRef+"..."+head)
	if err != nil {
		return err
	}
	tier := changemap.Mechanical
	for _, f := range touched {
		if t := changemap.Classify(f, cfg).Tier; t.Rank() > tier.Rank() {
			tier = t
		}
	}

	for _, m := range tierRe.FindAllStringSubmatch(tiersRe.FindString(p.Body), -1) {
		if t := changemap.Tier(m[1]); t.Rank() > tier.Rank() {
			tier = t
		}
	}
	r.Tier = string(tier)
	r.Required = level != "off" && tier.Rank() > changemap.TestsDocs.Rank()

	covers := func(sha string) (status string, changed []string, err error) {
		if _, err := run.Git(root, "rev-parse", "--verify", "--quiet", sha+"^{commit}"); err != nil {
			return "stale", nil, nil
		}
		if changed, err = changedSince(root, baseRef, sha, head, touched); err != nil {
			return "", nil, err
		}
		status = "current"
		for _, f := range changed {
			if changemap.Classify(f, cfg).Tier != changemap.TestsDocs {
				return "stale", changed, nil
			}
			status = "tests-docs"
		}
		return status, changed, nil
	}
	partial := false
	if r.Source != "none" {
		if r.Status, r.Changed, err = covers(r.SHA); err != nil {
			return err
		}

		if _, err := run.Git(root, "merge-base", "--is-ancestor", r.Base, mergeBase); err != nil && r.Status != "stale" {
			r.Status, partial = "stale", true
		}
	}
	say := note
	if r.Required {
		say = hold
	}
	switch r.Status {
	case "missing":
		if r.Required {
			hold("a required review never ran (level %s, tier %s): run /review %d", level, tier, p.Number)
		}
	case "stale":
		if partial {
			say("the review verdict of %s is stale: its range starts at %s, after the pull request's base, so it read only part of the change", short(r.SHA), short(r.Base))
			break
		}
		say("the review verdict of %s is stale: %s", short(r.SHA), describe(r.Changed, "reviewed"))
	case "tests-docs":
		say("only test or documentation files changed after the review verdict of %s: %s. Read their hunks; the review stands when they change no shipped behavior", short(r.SHA), describe(r.Changed, "reviewed"))
	}

	drivable, err := verifySkill(root, head)
	if err != nil {
		return err
	}
	rv.Required = tier.Rank() >= changemap.Behavioral.Rank() && drivable != ""
	if tier.Rank() >= changemap.Behavioral.Rank() && drivable == "" && rv.Source == "none" {
		note("no run-as-user verdict: the repository has no verify-<app> skill to drive the change; run /create-verification-skill to add one")
	}
	if rv.Source != "none" {
		if rv.Status, rv.Changed, err = covers(rv.SHA); err != nil {
			return err
		}
	}
	say = note
	if rv.Required {
		say = hold
	}
	switch rv.Status {
	case "missing":
		if rv.Required {
			hold("a required run-as-user verdict was never posted (tier %s): `verify-work` gets one from an agent that did not write the change", tier)
		}
	case "stale":

		say("the run-as-user verdict of %s is stale: %s", short(rv.SHA), describe(rv.Changed, "driven"))
	}
	return nil
}

func verifySkill(root, head string) (string, error) {
	out, err := run.Git(root, "ls-tree", "--name-only", head, "--", ".claude/skills/", ".agents/skills/", ".cursor/skills/", ".pi/skills/")
	if err != nil {
		return "", err
	}
	for _, entry := range strings.Split(out, "\n") {
		name := path.Base(entry)
		if !strings.HasPrefix(name, "verify-") || name == "verify-work" {
			continue
		}
		dir, kind, err := resolveTree(root, head, entry)
		if err != nil {
			return "", err
		}
		if kind != "tree" {
			continue
		}
		if _, kind, err = resolveTree(root, head, dir+"/SKILL.md"); err != nil {
			return "", err
		}
		if kind == "blob" {
			return name, nil
		}
	}
	return "", nil
}

func resolveTree(root, head, p string) (string, string, error) {
	for range 8 {
		if p == ".." || strings.HasPrefix(p, "../") {
			return p, "", nil
		}
		out, err := run.Git(root, "ls-tree", head, "--", p)
		if err != nil {
			return "", "", err
		}
		mode, rest, _ := strings.Cut(out, " ")
		kind, _, _ := strings.Cut(rest, " ")
		if out == "" || mode != "120000" {
			return p, kind, nil
		}
		target, err := run.Git(root, "cat-file", "-p", head+":"+p)
		if err != nil {
			return "", "", err
		}
		p = path.Clean(path.Join(path.Dir(p), target))
	}
	return p, "", nil
}

type posted struct {
	review.Verdict
	url string
}

func (e Env) verdicts(number int) (reviewed, drove *posted, ignored []string, err error) {
	raw, err := e.Forge("api", "--paginate", fmt.Sprintf("repos/{owner}/{repo}/issues/%d/comments?per_page=100", number))
	if err != nil {
		return nil, nil, nil, err
	}
	type comment struct {
		Body    string
		HTMLURL string `json:"html_url"`
		User    struct{ Login string }
	}
	permissions := map[string]string{}

	for dec := json.NewDecoder(bytes.NewReader(raw)); ; {
		var page []comment
		if err := dec.Decode(&page); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, nil, nil, fmt.Errorf("unreadable pull request comments: %w", err)
		}
		for _, cm := range page {
			v, ok := review.Parse(cm.Body)
			if !ok {
				continue
			}
			login := cm.User.Login
			if _, ok := permissions[login]; !ok {
				permissions[login] = e.permission(login)
			}
			if !slices.Contains([]string{"admin", "maintain", "write"}, permissions[login]) {
				ignored = append(ignored, fmt.Sprintf("%s by %s (permission %s)", cm.HTMLURL, login, permissions[login]))
				continue
			}
			if v.Run {
				drove = &posted{v, cm.HTMLURL}
			} else {
				reviewed = &posted{v, cm.HTMLURL}
			}
		}
	}
	return reviewed, drove, ignored, nil
}

var (
	changedSince = review.ChangedSince
	names        = review.Names
	patchID      = review.PatchID
)

func (e Env) view(number int) (pull, error) {
	args := []string{"pr", "view"}
	if number > 0 {
		args = append(args, strconv.Itoa(number))
	}
	var p pull
	err := e.read(&p, append(args, "--json", viewFields)...)
	return p, err
}

func (e Env) read(into any, args ...string) error {
	raw, err := e.Forge(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("gh %s: unreadable output: %w", strings.Join(args[:2], " "), err)
	}
	return nil
}

func describe(files []string, covered string) string {
	if len(files) == 0 {
		return "its " + covered + " commit is gone"
	}
	return strings.Join(files, ", ")
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
