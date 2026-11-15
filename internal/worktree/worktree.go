package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrRefused = errors.New("refused")

type Tree struct {
	Path     string
	Branch   string
	Head     string
	Main     bool
	Locked   bool
	Prunable bool
}

func List(dir string) ([]Tree, error) {
	out, err := git(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var trees []Tree
	for _, block := range strings.Split(out, "\n\n") {
		var t Tree
		for _, line := range strings.Split(block, "\n") {
			key, value, _ := strings.Cut(line, " ")
			switch key {
			case "worktree":
				t.Path = value
			case "HEAD":
				t.Head = value
			case "branch":
				t.Branch = strings.TrimPrefix(value, "refs/heads/")
			case "locked":
				t.Locked = true
			case "prunable":
				t.Prunable = true
			}
		}
		if t.Path != "" {
			t.Main = len(trees) == 0
			trees = append(trees, t)
		}
	}
	return trees, nil
}

type Status struct {
	Tree
	Changes []string
	Ignored []string
	Landed  bool
	Loose   bool
	State   string
}

func Inspect(t Tree, into string) (Status, error) {
	s := Status{Tree: t}
	if t.Prunable {
		s.State = "missing"
		return s, nil
	}
	out, err := gitRaw(t.Path, "status", "--porcelain", "--ignored")
	if err != nil {
		return s, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(line, "!! "); ok {
			s.Ignored = append(s.Ignored, rest)
		} else if line != "" {
			s.Changes = append(s.Changes, line)
		}
	}
	if t.Branch != "" {
		s.Landed = Landed(t.Path, "refs/heads/"+t.Branch, into)
	} else {
		held, err := git(t.Path, "for-each-ref", "--count=1", "--contains", t.Head)
		if err != nil {
			return s, err
		}
		s.Loose = held == ""
	}
	switch {
	case t.Locked:
		s.State = "locked"
	case len(s.Changes) > 0:
		s.State = "dirty"
	case s.Loose:
		s.State = "loose"
	case t.Branch != "" && !s.Landed:
		s.State = "unlanded"
	case len(s.Ignored) > 0:
		s.State = "ignored"
	default:
		s.State = "safe"
	}
	return s, nil
}

func Landed(dir, rev, into string) bool {
	if _, err := git(dir, "merge-base", "--is-ancestor", rev, into); err == nil {
		return true
	}
	merged, err := git(dir, "merge-tree", "--write-tree", into, rev)
	if err != nil {
		return false
	}
	tree, err := git(dir, "rev-parse", "--verify", into+"^{tree}")
	first, _, _ := strings.Cut(merged, "\n")
	return err == nil && first == tree
}

func DefaultInto(dir string) (string, error) {
	head, _ := git(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	for _, ref := range []string{head, "origin/main", "origin/master"} {
		if ref == "" {
			continue
		}
		if _, err := git(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err == nil {
			return ref, nil
		}
	}
	return "", errors.New("no default branch ref (origin/HEAD, origin/main, origin/master); pass --into <ref>")
}

type Options struct {
	Into         string
	At           string
	SavePatch    string
	AllowIgnored bool
	KeepBranch   bool
}

func Retire(dir, path string, o Options) ([]string, error) {
	trees, err := List(dir)
	if err != nil {
		return nil, err
	}
	var tree *Tree
	for i := range trees {
		if samePath(trees[i].Path, path) {
			tree = &trees[i]
		}
	}
	switch {
	case tree == nil:
		return nil, fmt.Errorf("%w: %s is not a worktree of this repository", ErrRefused, path)
	case tree.Main:
		return nil, fmt.Errorf("%w: %s is the main worktree, which is never removed", ErrRefused, path)
	case tree.Prunable:
		return nil, fmt.Errorf("%w: %s is already gone; run git worktree prune", ErrRefused, path)
	}
	s, err := Inspect(*tree, o.Into)
	if err != nil {
		return nil, err
	}

	o.KeepBranch = o.KeepBranch || isInto(s.Branch, o.Into)
	if s.Locked {
		return nil, fmt.Errorf("%w: %s is locked; unlock it first", ErrRefused, path)
	}
	if s.Branch != "" && !o.KeepBranch && o.Into == "" {
		return nil, fmt.Errorf("%w: no ref to prove branch %s landed on; pass --into <ref>", ErrRefused, s.Branch)
	}
	if s.Branch != "" && !o.KeepBranch && !s.Landed {
		return nil, fmt.Errorf("%w: branch %s is not proven landed on %s; pass --keep-branch to remove only the worktree", ErrRefused, s.Branch, o.Into)
	}

	moved := s.Loose && s.Head != o.At
	if moved && o.At == "" {
		return nil, fmt.Errorf("%w: detached at %s, which no ref holds; pass --at <sha> naming the commit the worktree was created at", ErrRefused, s.Head)
	}
	save := len(s.Changes) > 0 || moved
	if save && o.SavePatch == "" {
		if moved {
			return nil, fmt.Errorf("%w: detached head moved from %s to %s, which no ref holds; pass --save-patch <file> to keep its commits", ErrRefused, o.At, s.Head)
		}
		return nil, fmt.Errorf("%w: uncommitted work; commit it, or pass --save-patch <file> to keep it:\n%s", ErrRefused, strings.Join(s.Changes, "\n"))
	}
	if len(s.Ignored) > 0 && !o.AllowIgnored {
		return nil, fmt.Errorf("%w: removal deletes these ignored paths; pass --allow-ignored once each is regenerable build output, never local configuration or a secret:\n%s", ErrRefused, strings.Join(s.Ignored, "\n"))
	}

	var done []string
	remove := []string{"worktree", "remove", s.Path}
	if save {
		from := o.At
		if from == "" {
			from = "HEAD"
		}
		if _, err := git(s.Path, "add", "-A", "."); err != nil {
			return nil, err
		}
		patch, err := gitRaw(s.Path, "diff", "--cached", "--binary", from)
		if err != nil {
			return nil, err
		}
		if len(patch) == 0 {
			return nil, fmt.Errorf("%w: the worktree's work produced an empty patch", ErrRefused)
		}
		if err := os.WriteFile(o.SavePatch, patch, 0o644); err != nil {
			return nil, err
		}
		done = append(done, "saved patch "+o.SavePatch)
		remove = []string{"worktree", "remove", "--force", s.Path}
	}

	if _, err := git(trees[0].Path, remove...); err != nil {
		return done, err
	}
	done = append(done, "removed worktree "+s.Path)
	if s.Branch == "" {
		return done, nil
	}
	if o.KeepBranch {
		return append(done, "kept branch "+s.Branch), nil
	}
	if _, err := git(trees[0].Path, "branch", "-D", s.Branch); err != nil {
		return done, err
	}
	return append(done, "deleted branch "+s.Branch), nil
}

func RetireBranch(dir, branch, into string) ([]string, error) {
	trees, err := List(dir)
	if err != nil {
		return nil, err
	}
	for _, t := range trees {
		if t.Branch == branch {
			return nil, fmt.Errorf("%w: worktree %s holds branch %s; retire that worktree instead", ErrRefused, t.Path, branch)
		}
	}
	if _, err := git(dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		return nil, nil
	}
	if isInto(branch, into) {
		return nil, fmt.Errorf("%w: %s is the branch --into names", ErrRefused, branch)
	}
	if !Landed(dir, "refs/heads/"+branch, into) {
		return nil, fmt.Errorf("%w: branch %s is not proven landed on %s", ErrRefused, branch, into)
	}
	if _, err := git(trees[0].Path, "branch", "-D", branch); err != nil {
		return nil, err
	}
	return []string{"deleted branch " + branch}, nil
}

func isInto(branch, into string) bool {
	return branch != "" && (into == branch || strings.HasSuffix(into, "/"+branch))
}

func samePath(a, b string) bool {
	resolve := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		return filepath.Clean(p)
	}
	return resolve(a) == resolve(b)
}

func git(dir string, args ...string) (string, error) {
	out, err := gitRaw(dir, args...)
	return strings.TrimSpace(string(out)), err
}

func gitRaw(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
