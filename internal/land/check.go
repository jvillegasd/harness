// Package land answers whether a pull request can land, for `harness land check`. It is the one place the land skill's lane, mode, gate, and review-coverage rules live; the command reads the forge and the repository and writes nothing but fetched refs.
package land

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jvillegasd/harness/internal/changemap"
	"github.com/jvillegasd/harness/internal/reviewledger"
	"github.com/jvillegasd/harness/internal/worktree"
)

// Forge runs one gh command and returns its stdout, which a caller may still read when the command failed.
type Forge func(args ...string) ([]byte, error)

// Env is what a check reads from: a directory of the repository, the forge, and the state file holding the review level.
type Env struct {
	Dir   string
	Forge Forge
	State string
}

// Check is the verdict for one pull request. A non-empty Holds stops an open pull request from merging; for a merged one it lists what the merge skipped.
type Check struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Lane        string `json:"lane"`           // open, merged, or stop
	Mode        string `json:"mode,omitempty"` // local, remote, or fork
	Mine        bool   `json:"mine"`           // the forge user authored it
	Head        string `json:"head"`
	HeadRefName string `json:"headRefName"`
	Base        string `json:"base"`
	Default     string `json:"default"`
	// After is the pull request earlier in this run whose head is this one's base.
	After int `json:"after,omitempty"`
	// Key names the review directory: the head branch, or pr-<n> for a fork.
	Key string `json:"key"`
	// Worktrees are the other checkouts holding the head branch, which a landing leaves alone.
	Worktrees  []string    `json:"worktrees"`
	Review     *Review     `json:"review,omitempty"`
	Findings   []Finding   `json:"findings"`
	Dropped    int         `json:"dropped"`
	Dependents []Dependent `json:"dependents"`
	Holds      []string    `json:"holds"`
	Notes      []string    `json:"notes"`
	// gate is what the forge gates found, which `harness pr status` ranks into its exit code.
	gate gateFacts
}

// gateFacts sorts the forge gates' holds into the blockers `harness pr status` names, and keeps the unresolved threads it lists.
type gateFacts struct {
	conflict, failing, pending bool
	threads                    []Thread
}

// Thread is one unresolved review thread, located by its first comment.
type Thread struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Author string `json:"author"`
	// Permission is the author's permission on the base repository: admin, maintain, write, triage, read, none, or unknown when the forge would not say.
	Permission string `json:"permission"`
	Body       string `json:"body"`
	URL        string `json:"url"`
}

// Review is whether a review is required and whether one still covers the head.
type Review struct {
	Required bool   `json:"required"`
	Level    string `json:"level"`
	Tier     string `json:"tier"`
	Source   string `json:"source"` // body, ledger, approval, or none
	SHA      string `json:"sha,omitempty"`
	Status   string `json:"status"` // current, tests-docs, stale, or missing
	// Changed are the files whose patch differs between the reviewed commit and the head.
	Changed []string `json:"changed,omitempty"`
}

// Finding is a ledger finding that holds a merge: open, or dismissed on a recorded word.
type Finding struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Lines       string `json:"lines,omitempty"`
	Severity    string `json:"severity"`
	Summary     string `json:"summary"`
	Disposition string `json:"disposition"`
	Evidence    string `json:"evidence,omitempty"`
}

// Dependent is an open pull request based on this one's head branch.
type Dependent struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	// Mine is false for another author's or a fork's branch, which a landing reports instead of rebasing.
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

// Run resolves the pull requests a landing covers and checks each, in the order they land. No number means the pull request of the branch checked out in e.Dir. With stack, the one number is a stack's tip and every open pull request under it joins.
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

// landing is the resolved run: the repository root, the default branch, the forge user, and the pull requests in the order they land.
type landing struct {
	root, def, me string
	pulls         []pull
}

// below is the open pull request among the given ones whose head is p's base, which p lands after.
func (landing) below(p pull, among []pull) int {
	for _, q := range among {
		if q.Number != p.Number && q.State == "OPEN" && !q.IsCrossRepository && q.HeadRefName == p.BaseRefName {
			return q.Number
		}
	}
	return 0
}

// resolve reads the forge user, the default branch, and the pull requests a run covers, walking a stack down from its tip, and orders them bottom-up.
func (e Env) resolve(numbers []int, stack bool) (landing, error) {
	var run landing
	root, err := git(e.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return run, errors.New("not inside a git repository")
	}
	var repo struct{ DefaultBranchRef struct{ Name string } }
	if err := e.read(&repo, "repo", "view", "--json", "defaultBranchRef"); err != nil {
		return run, err
	}
	def := repo.DefaultBranchRef.Name
	raw, err := e.Forge("api", "user", "--jq", ".login")
	me := strings.TrimSpace(string(raw))
	if err != nil || me == "" || def == "" {
		return run, fmt.Errorf("cannot read the forge user and default branch: %v", err)
	}

	var pulls []pull
	if len(numbers) == 0 {
		numbers = []int{0}
	}
	for _, n := range numbers {
		p, err := e.view(n)
		if err != nil {
			return run, err
		}
		if !slices.ContainsFunc(pulls, func(q pull) bool { return q.Number == p.Number }) {
			pulls = append(pulls, p)
		}
	}
	// A stack is walked down from its tip: each base that is not the default branch must be the head of an open pull request in this repository.
	for p := pulls[0]; stack && p.State == "OPEN" && p.BaseRefName != def && len(pulls) < 50; {
		var heads []struct {
			Number            int
			IsCrossRepository bool
		}
		if err := e.read(&heads, "pr", "list", "--state", "open", "--head", p.BaseRefName, "--json", "number,isCrossRepository"); err != nil {
			return run, err
		}
		under := 0
		for _, h := range heads {
			if !h.IsCrossRepository {
				under = h.Number
			}
		}
		if under == 0 {
			return run, fmt.Errorf("#%d is based on %s, which is not the default branch and has no open pull request", p.Number, p.BaseRefName)
		}
		if p, err = e.view(under); err != nil {
			return run, err
		}
		pulls = append(pulls, p)
	}

	// A pull request lands after the open one in the run whose head is its base.
	var ordered []pull
	for rest := pulls; len(rest) > 0; {
		var blocked []pull
		for _, p := range rest {
			if run.below(p, rest) != 0 {
				blocked = append(blocked, p)
			} else {
				ordered = append(ordered, p)
			}
		}
		if len(blocked) == len(rest) {
			return run, errors.New("the pull requests are based on each other in a cycle")
		}
		rest = blocked
	}

	return landing{root, def, me, ordered}, nil
}

func (e Env) check(root string, p pull, def, me string, merge bool, after int) (Check, error) {
	c := Check{
		Number: p.Number, URL: p.URL, Title: p.Title, Mine: p.Author.Login == me,
		Head: p.HeadRefOid, HeadRefName: p.HeadRefName, Base: p.BaseRefName, Default: def, After: after, Key: p.HeadRefName,
		Worktrees: []string{}, Findings: []Finding{}, Dependents: []Dependent{}, Holds: []string{}, Notes: []string{},
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

	// refs/pull/<n>/head exists for every pull request, a fork's and a merged one's included, so the forge's head is what every comparison below reads.
	if _, err := git(root, "fetch", "--quiet", "origin", fmt.Sprintf("refs/pull/%d/head", p.Number)); err != nil {
		return c, err
	}
	if _, err := git(root, "cat-file", "-e", p.HeadRefOid+"^{commit}"); err != nil {
		return c, fmt.Errorf("the fetched pull request does not hold its head %s; run again", p.HeadRefOid)
	}
	baseRef := "origin/" + p.BaseRefName
	if _, err := git(root, "fetch", "--quiet", "origin", p.BaseRefName); err != nil {
		baseRef = ""
	} else if _, err := git(root, "merge-base", "--is-ancestor", p.HeadRefOid, baseRef); err == nil && p.MergeCommit.Oid != "" {
		// A merge commit put the head itself on the base, so the change is read against the base as it was before that merge.
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
		// A fork's branch lives in another repository, so a local branch of its name is someone else's.
		case p.IsCrossRepository || t.Branch != p.HeadRefName:
		case t.Path != root:
			c.Worktrees = append(c.Worktrees, t.Path)
		case t.Head == p.HeadRefOid:
			c.Mode = "local"
		case open:
			hold("this checkout's %s is at %s but the pull request's head is %s: push or reset it, or land from another checkout", t.Branch, short(t.Head), short(p.HeadRefOid))
		default:
			// A merged head the checkout is behind is still its branch; anything else is a reused name or unpushed work.
			if _, err := git(root, "merge-base", "--is-ancestor", t.Head, p.HeadRefOid); err == nil {
				c.Mode = "local"
			} else {
				note("this checkout's %s is not this pull request's head (a reused name or unpushed commits), so it is left alone", t.Branch)
			}
		}
	}

	if open {
		e.gates(p, &c, def)
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

// gates adds a hold for everything the forge says stands between an open pull request and its merge.
func (e Env) gates(p pull, c *Check, def string) {
	hold := func(format string, args ...any) { c.Holds = append(c.Holds, fmt.Sprintf(format, args...)) }
	if p.BaseRefName != def && c.After == 0 {
		hold("its base %s is not the default branch, so merging would squash it into that base: land the base's pull request first, or retarget it with gh pr edit %d --base %s once that base has merged", p.BaseRefName, p.Number, def)
	}
	// UNSTABLE is a failing check that is not required, and HAS_HOOKS a mergeable branch with hooks.
	switch {
	case p.Mergeable == "UNKNOWN" || p.MergeStateStatus == "UNKNOWN":
		hold("the forge is still computing whether it can merge: check again")
	case p.Mergeable == "CONFLICTING" || p.MergeStateStatus == "DIRTY":
		c.gate.conflict = true
		hold("the forge reports it %s, not MERGEABLE", p.Mergeable)
	case p.Mergeable != "MERGEABLE":
		hold("the forge reports it %s, not MERGEABLE", p.Mergeable)
	case !slices.Contains([]string{"CLEAN", "UNSTABLE", "HAS_HOOKS"}, p.MergeStateStatus):
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
	// gh exits non-zero while a check is pending or failing and still prints them, and reports a branch with no required checks as an error.
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
		// An outdated thread has no line on the current diff, so it is placed where it was written.
		th := Thread{ID: t.ID, Path: t.Path, Line: t.Line}
		if th.Line == 0 {
			th.Line = t.OriginalLine
		}
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
	reviewedRe = regexp.MustCompile(`(?m)^Reviewed:\s*[0-9a-f]{7,40}\.\.([0-9a-f]{7,40})\s*$`)
	tiersRe    = regexp.MustCompile(`(?m)^Tiers:.*$`)
	tierRe     = regexp.MustCompile(`(critical|behavioral|structural)\s+[1-9]`)
)

// review decides whether a review is required, finds the one that claims to cover the pull request, compares what it read with the head, and adds the ledger's open findings.
func (e Env) review(root string, p pull, c *Check, baseRef string) error {
	hold := func(format string, args ...any) { c.Holds = append(c.Holds, fmt.Sprintf(format, args...)) }
	note := func(format string, args ...any) { c.Notes = append(c.Notes, fmt.Sprintf(format, args...)) }
	level, _, err := reviewledger.LoadLevel(e.State)
	if err != nil {
		return err
	}
	r := &Review{Level: level, Source: "none", Status: "missing"}
	c.Review = r
	head := p.HeadRefOid

	file, err := reviewledger.File(root, c.Key)
	if err != nil {
		return err
	}
	entries, err := reviewledger.Read(file)
	if err != nil {
		return err
	}
	latest := reviewledger.Latest(entries, c.Key, func(sha string) bool {
		_, err := git(root, "merge-base", "--is-ancestor", sha, head)
		return err == nil
	})
	if len(latest) == 0 {
		note("no review ledger for %s on this machine, so its findings are unknown here", c.Key)
	}
	for _, f := range latest {
		path := ""
		if f.Path != nil {
			path = *f.Path
		}
		switch {
		case f.Open:
			hold("review finding %s is open: %s %s", f.ID, path, f.Summary)
		case f.Disposition == "accepted":
			// The ledger is a local file any process can write, so a recorded dismissal is shown to the human again.
			hold("review finding %s was dismissed and needs the human's word again: %s %s", f.ID, path, f.Summary)
		case f.Disposition == "dropped":
			c.Dropped++
			continue
		default:
			continue
		}
		c.Findings = append(c.Findings, Finding{f.ID, path, f.Lines, f.Severity, f.Summary, f.Disposition, f.Evidence})
	}

	if baseRef == "" {
		r.Required, r.Status = level != "off", "stale"
		hold("its base branch %s is gone from origin, so no review can be compared with what merged", p.BaseRefName)
		return nil
	}
	mergeBase, err := git(root, "merge-base", baseRef, head)
	if err != nil {
		return err
	}
	cfg, err := changemap.EffectiveConfig(root, mergeBase)
	if err != nil {
		return err
	}
	// A locked skill body collapses to mechanical only while it matches its lock, which only the checkout that holds the head can prove.
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
	// The body's own map may raise a tier above the floor; it never lowers it.
	for _, m := range tierRe.FindAllStringSubmatch(tiersRe.FindString(p.Body), -1) {
		if t := changemap.Tier(m[1]); t.Rank() > tier.Rank() {
			tier = t
		}
	}
	r.Tier = string(tier)
	r.Required = level != "off" && tier.Rank() > changemap.TestsDocs.Rank()

	if m := reviewedRe.FindStringSubmatch(p.Body); m != nil {
		r.Source, r.SHA = "body", m[1]
	} else {
		for _, f := range latest {
			if f.Disposition == "clean" && f.Depth != "none" {
				r.Source, r.SHA = "ledger", f.SHA
			}
		}
	}
	// Someone else's pull request may rest on a person's approval; the forge user's own keeps its blind review.
	if r.Source == "none" && !c.Mine {
		if r.SHA, err = e.approval(p.Number); err != nil {
			return err
		}
		if r.SHA != "" {
			r.Source = "approval"
		}
	}

	if r.Source != "none" {
		// A reviewed commit that is gone cannot be compared, so its review is stale.
		r.Status = "stale"
		if full, err := git(root, "rev-parse", "--verify", "--quiet", r.SHA+"^{commit}"); err == nil {
			r.SHA, r.Status = full, "current"
			if r.Changed, err = changedSince(root, baseRef, full, head, touched); err != nil {
				return err
			}
			for _, f := range r.Changed {
				if changemap.Classify(f, cfg).Tier != changemap.TestsDocs {
					r.Status = "stale"
					break
				}
				r.Status = "tests-docs"
			}
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
		say("the %s review of %s is stale: %s", r.Source, short(r.SHA), describe(r.Changed))
	case "tests-docs":
		say("only test or documentation files changed after the %s review of %s: %s. Read their hunks; the review stands when they change no shipped behavior", r.Source, short(r.SHA), describe(r.Changed))
	}
	return nil
}

// approval returns the commit a person last approved, or nothing when no person approved or any person's latest review requests changes.
func (e Env) approval(number int) (string, error) {
	raw, err := e.Forge("api", "--paginate", fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/reviews?per_page=100", number))
	if err != nil {
		return "", err
	}
	type forgeReview struct {
		State    string
		CommitID string `json:"commit_id"`
		User     struct{ Login, Type string }
	}
	verdicts := map[string]forgeReview{}
	last := ""
	// A paginated response is one JSON array per page.
	for dec := json.NewDecoder(bytes.NewReader(raw)); ; {
		var page []forgeReview
		if err := dec.Decode(&page); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return "", fmt.Errorf("unreadable pull request reviews: %w", err)
		}
		for _, v := range page {
			if v.User.Type == "User" && (v.State == "APPROVED" || v.State == "CHANGES_REQUESTED") {
				verdicts[v.User.Login] = v
				last = v.User.Login
			}
		}
	}
	for _, v := range verdicts {
		if v.State == "CHANGES_REQUESTED" {
			return "", nil
		}
	}
	// Reviews arrive oldest first, so the last verdict read is the newest approval.
	return verdicts[last].CommitID, nil
}

// The patch comparison is the review ledger's, so land check and review covers read the same change the same way.
var (
	changedSince = reviewledger.ChangedSince
	names        = reviewledger.Names
	patchID      = reviewledger.PatchID
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

func describe(files []string) string {
	if len(files) == 0 {
		return "its reviewed commit is gone"
	}
	return strings.Join(files, ", ")
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
