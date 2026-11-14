package land

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jvillegasd/harness/internal/reviewledger"
	"github.com/jvillegasd/harness/internal/worktree"
)

// Action is one step of a cleanup: done, or kept with the reason.
type Action struct {
	Done bool
	Text string
}

func (a Action) String() string {
	if a.Done {
		return "done  " + a.Text
	}
	return "kept  " + a.Text
}

// ErrDiverged marks a local default branch that cannot fast-forward to the remote's.
var ErrDiverged = errors.New("cannot fast-forward")

// defaultBranch reads the repository root and the forge's default branch.
func (e Env) defaultBranch() (root, def string, err error) {
	root, err = git(e.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", errors.New("not inside a git repository")
	}
	var repo struct{ DefaultBranchRef struct{ Name string } }
	if err := e.read(&repo, "repo", "view", "--json", "defaultBranchRef"); err != nil {
		return "", "", err
	}
	if repo.DefaultBranchRef.Name == "" {
		return "", "", errors.New("the forge reports no default branch")
	}
	return root, repo.DefaultBranchRef.Name, nil
}

// Sync fetches from origin with pruning and fast-forwards the local default branch: in the worktree that holds it, or by moving the branch when none does. A default branch that cannot fast-forward is left as it is and reported as ErrDiverged.
func Sync(e Env) (Action, error) {
	root, def, err := e.defaultBranch()
	if err != nil {
		return Action{}, err
	}
	return sync(root, def)
}

func sync(root, def string) (Action, error) {
	if _, err := git(root, "fetch", "--quiet", "--prune", "origin"); err != nil {
		return Action{}, err
	}
	remote, err := git(root, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+def)
	if err != nil {
		return Action{}, fmt.Errorf("origin has no %s", def)
	}
	local, err := git(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+def)
	if err != nil {
		return Action{false, fmt.Sprintf("%s: no local branch to fast-forward", def)}, nil
	}
	if local == remote {
		return Action{false, fmt.Sprintf("%s: already at origin/%s %s", def, def, short(remote))}, nil
	}
	if _, err := git(root, "merge-base", "--is-ancestor", local, remote); err != nil {
		return Action{false, fmt.Sprintf("%s: local %s is not behind origin/%s %s", def, short(local), def, short(remote))}, fmt.Errorf("%w: %s has commits origin/%s lacks; it was left as it is", ErrDiverged, def, def)
	}
	trees, err := worktree.List(root)
	if err != nil {
		return Action{}, err
	}
	for _, t := range trees {
		if t.Branch == def {
			if _, err := git(t.Path, "merge", "--ff-only", "--quiet", "origin/"+def); err != nil {
				return Action{false, fmt.Sprintf("%s: the fast-forward in %s failed", def, t.Path)}, err
			}
			return Action{true, fmt.Sprintf("fast-forwarded %s to %s in %s", def, short(remote), t.Path)}, nil
		}
	}
	// No checkout holds it, so the ref moves alone, and only from the value just read.
	if _, err := git(root, "update-ref", "refs/heads/"+def, remote, local); err != nil {
		return Action{}, err
	}
	return Action{true, fmt.Sprintf("fast-forwarded %s to %s", def, short(remote))}, nil
}

// FinishOptions are the caller's answers for retiring the checkout: whether its ignored paths may go, and whether its branch stays.
type FinishOptions struct {
	AllowIgnored bool
	KeepBranch   bool
}

// Finish cleans up after merged pull requests: it syncs the default branch, then for each removes its review directory, deletes its remote head branch when that is safe, and retires its local branch, retiring this checkout once, last. It returns every action taken or kept; an error after the first action still returns them. A pull request that is not merged is refused before anything is removed.
func Finish(e Env, numbers []int, o FinishOptions) ([]Action, error) {
	root, def, err := e.defaultBranch()
	if err != nil {
		return nil, err
	}
	raw, err := e.Forge("api", "user", "--jq", ".login")
	me := strings.TrimSpace(string(raw))
	if err != nil || me == "" {
		return nil, fmt.Errorf("cannot read the forge user: %v", err)
	}
	var pulls []pull
	for _, n := range numbers {
		p, err := e.view(n)
		if err != nil {
			return nil, err
		}
		if p.State != "MERGED" {
			return nil, fmt.Errorf("#%d is %s, not merged; nothing was removed", p.Number, p.State)
		}
		pulls = append(pulls, p)
	}

	var actions []Action
	a, err := sync(root, def)
	actions = append(actions, a)
	// A diverged default branch is reported and the cleanup goes on, since nothing below reads it.
	if err != nil && !errors.Is(err, ErrDiverged) {
		return actions, err
	}
	into := "origin/" + def
	trees, err := worktree.List(root)
	if err != nil {
		return actions, err
	}
	var here *worktree.Tree
	for _, t := range trees {
		if t.Path == root {
			here = &t
		}
	}

	var last *pull
	for i, p := range pulls {
		key := p.HeadRefName
		if p.IsCrossRepository {
			key = fmt.Sprintf("pr-%d", p.Number)
		}
		file, err := reviewledger.File(root, key)
		if err != nil {
			return actions, err
		}
		switch removed, err := reviewledger.RemoveBranch(file); {
		case errors.Is(err, reviewledger.ErrHeld):
			actions = append(actions, Action{false, fmt.Sprintf("#%d review directory: %v", p.Number, err)})
		case err != nil:
			return actions, err
		case removed:
			actions = append(actions, Action{true, fmt.Sprintf("#%d removed review directory %s", p.Number, filepath.Dir(file))})
		}
		if p.IsCrossRepository {
			actions = append(actions, Action{false, fmt.Sprintf("#%d comes from a fork, so its branches are not this repository's", p.Number)})
			continue
		}

		a, err := e.deleteRemote(root, p, me)
		if err != nil {
			return actions, err
		}
		if a.Text != "" {
			actions = append(actions, a)
		}

		if here != nil && here.Branch == p.HeadRefName {
			last = &pulls[i]
			continue
		}
		done, err := worktree.RetireBranch(root, p.HeadRefName, into)
		for _, d := range done {
			actions = append(actions, Action{true, fmt.Sprintf("#%d %s", p.Number, d)})
		}
		if err != nil {
			actions = append(actions, Action{false, fmt.Sprintf("#%d local branch %s: %v", p.Number, p.HeadRefName, err)})
		}
	}

	// Retiring this checkout removes the directory the session runs in, so it comes once, after everything else.
	if last != nil {
		retired, err := e.retireHere(root, *here, def, into, o)
		for _, d := range retired {
			actions = append(actions, Action{true, fmt.Sprintf("#%d %s", last.Number, d)})
		}
		if err != nil {
			actions = append(actions, Action{false, fmt.Sprintf("#%d this checkout %s: %v", last.Number, root, err)})
			return actions, err
		}
	}
	return actions, nil
}

// deleteRemote deletes the pull request's head branch on origin when it is the forge user's and no open pull request is based on it, and says why when it stays.
func (e Env) deleteRemote(root string, p pull, me string) (Action, error) {
	if _, err := git(root, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+p.HeadRefName); err != nil {
		return Action{}, nil
	}
	if p.Author.Login != me {
		return Action{false, fmt.Sprintf("#%d remote branch %s: it is %s's", p.Number, p.HeadRefName, p.Author.Login)}, nil
	}
	var deps []struct{ Number int }
	if err := e.read(&deps, "pr", "list", "--state", "open", "--base", p.HeadRefName, "--json", "number"); err != nil {
		return Action{}, err
	}
	if len(deps) > 0 {
		var ns []string
		for _, d := range deps {
			ns = append(ns, fmt.Sprintf("#%d", d.Number))
		}
		return Action{false, fmt.Sprintf("#%d remote branch %s: %s still based on it", p.Number, p.HeadRefName, strings.Join(ns, ", "))}, nil
	}
	if _, err := git(root, "push", "--quiet", "origin", "--delete", p.HeadRefName); err != nil {
		return Action{}, err
	}
	return Action{true, fmt.Sprintf("#%d deleted remote branch %s", p.Number, p.HeadRefName)}, nil
}

// retireHere retires this checkout and its branch. The main worktree is never removed, so there the branch goes once the checkout switches to the default branch.
func (e Env) retireHere(root string, here worktree.Tree, def, into string, o FinishOptions) ([]string, error) {
	if !here.Main {
		return worktree.Retire(root, root, worktree.Options{Into: into, AllowIgnored: o.AllowIgnored, KeepBranch: o.KeepBranch})
	}
	if o.KeepBranch {
		return []string{"kept branch " + here.Branch + " in the main worktree"}, nil
	}
	if _, err := git(root, "switch", "--quiet", def); err != nil {
		return nil, err
	}
	done, err := worktree.RetireBranch(root, here.Branch, into)
	return append([]string{"switched the main worktree to " + def}, done...), err
}
