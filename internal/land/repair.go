package land

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jvillegasd/harness/internal/worktree"
)

var ErrRepairRefused = errors.New("refused")

type Repair struct {
	Number    int    `json:"number"`
	Branch    string `json:"branch"`
	Default   string `json:"default"`
	Boundary  string `json:"boundary"`
	Lease     string `json:"lease"`
	Rebased   string `json:"rebased"`
	Worktree  string `json:"worktree"`
	Temporary bool   `json:"temporary"`
}

func repairFile(root string, number int) (string, error) {
	common, err := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "harness-land", "repair", strconv.Itoa(number)+".json"), nil
}

func RepairDependent(e Env, dependent, landed int) (Repair, error) {
	refuse := func(format string, a ...any) (Repair, error) {
		return Repair{}, fmt.Errorf("%w: #%d: %s", ErrRepairRefused, dependent, fmt.Sprintf(format, a...))
	}
	root, def, err := e.defaultBranch()
	if err != nil {
		return Repair{}, err
	}
	raw, err := e.Forge("api", "user", "--jq", ".login")
	me := strings.TrimSpace(string(raw))
	if err != nil || me == "" {
		return Repair{}, fmt.Errorf("cannot read the forge user: %v", err)
	}
	d, err := e.view(dependent)
	if err != nil {
		return Repair{}, err
	}
	l, err := e.view(landed)
	if err != nil {
		return Repair{}, err
	}
	switch {
	case l.State != "MERGED":
		return refuse("#%d is %s, not merged", landed, l.State)
	case d.State != "OPEN":
		return refuse("it is %s", d.State)
	case d.IsCrossRepository:
		return refuse("it comes from a fork, whose branch is its author's to rebase")
	case d.Author.Login != me:
		return refuse("it is %s's, whose branch is theirs to rebase", d.Author.Login)
	}
	for _, ref := range []string{def, fmt.Sprintf("refs/pull/%d/head", landed), fmt.Sprintf("refs/pull/%d/head", dependent)} {
		if _, err := git(root, "fetch", "--quiet", "origin", ref); err != nil {
			return Repair{}, err
		}
	}
	onto := "refs/remotes/origin/" + def
	lease, landedHead := d.HeadRefOid, l.HeadRefOid
	for _, sha := range []string{lease, landedHead} {
		if _, err := git(root, "cat-file", "-e", sha+"^{commit}"); err != nil {
			return Repair{}, fmt.Errorf("the fetched pull requests do not hold %s; run again", sha)
		}
	}

	if merges, err := git(root, "rev-list", "--merges", onto+".."+lease); err != nil {
		return Repair{}, err
	} else if merges != "" {
		return refuse("it holds a merge commit, so its own commits cannot be told apart: %s", short(strings.Fields(merges)[0]))
	}

	boundary, err := findBoundary(root, landedHead, lease)
	if err != nil {
		return Repair{}, err
	}
	if boundary == "" {
		return refuse("none of its commits are already in #%d's head, so the commit it was built on is unknown", landed)
	}

	merged, err := git(root, "merge-tree", "--write-tree", onto, boundary)
	tree, _ := git(root, "rev-parse", onto+"^{tree}")
	if first, _, _ := strings.Cut(merged, "\n"); err != nil || first != tree {
		return refuse("its commits up to %s are not all on %s, so rebasing from there would drop its own work", short(boundary), def)
	}
	own, err := filePatches(root, boundary, lease)
	if err != nil {
		return Repair{}, err
	}
	if len(own) == 0 {
		return refuse("it changes nothing past %s", short(boundary))
	}

	trees, err := worktree.List(root)
	if err != nil {
		return Repair{}, err
	}
	r := Repair{Number: dependent, Branch: d.HeadRefName, Default: def, Boundary: boundary, Lease: lease}
	for _, t := range trees {
		if t.Branch != d.HeadRefName {
			continue
		}
		if t.Head != lease {
			return refuse("its checkout %s is at %s, not the pull request's head %s; push or reset it first", t.Path, short(t.Head), short(lease))
		}
		if status, err := git(t.Path, "status", "--porcelain"); err != nil || status != "" {
			return refuse("its checkout %s has uncommitted changes", t.Path)
		}
		r.Worktree = t.Path
	}

	if _, err := e.Forge("pr", "edit", strconv.Itoa(dependent), "--base", def); err != nil {
		return Repair{}, err
	}
	if r.Worktree == "" {
		parent, err := os.MkdirTemp("", "harness-repair-")
		if err != nil {
			return Repair{}, err
		}
		r.Worktree, r.Temporary = filepath.Join(parent, "dependent"), true
		if _, err := git(root, "-c", "core.hooksPath=/dev/null", "worktree", "add", "--quiet", "--detach", r.Worktree, lease); err != nil {
			os.RemoveAll(parent)
			return Repair{}, err
		}
	}

	undo := func() {
		e.Forge("pr", "edit", strconv.Itoa(dependent), "--base", d.BaseRefName)
		if r.Temporary {
			git(root, "worktree", "remove", "--force", r.Worktree)
			os.RemoveAll(filepath.Dir(r.Worktree))
		} else {
			git(r.Worktree, "reset", "--quiet", "--hard", lease)
		}
	}
	if _, err := git(r.Worktree, "-c", "core.hooksPath=/dev/null", "rebase", "--quiet", "--onto", onto, boundary); err != nil {
		git(r.Worktree, "rebase", "--abort")
		undo()
		return Repair{}, fmt.Errorf("the rebase onto %s conflicted, so the branch was left as it was: %w", def, err)
	}
	if r.Rebased, err = git(r.Worktree, "rev-parse", "HEAD"); err != nil {
		undo()
		return Repair{}, err
	}
	after, err := filePatches(root, onto, r.Rebased)
	if err != nil {
		undo()
		return Repair{}, err
	}
	if differ := differing(own, after); len(differ) > 0 {
		undo()
		return refuse("its own change came out different after the rebase, so the branch was put back: %s", strings.Join(differ, ", "))
	}

	file, err := repairFile(root, dependent)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(file), 0o755)
	}
	if err == nil {
		raw, _ := json.MarshalIndent(r, "", "  ")
		err = os.WriteFile(file, raw, 0o644)
	}
	if err != nil {
		undo()
		return Repair{}, err
	}
	return r, nil
}

func findBoundary(root, landedHead, lease string) (string, error) {
	if _, err := git(root, "merge-base", "--is-ancestor", landedHead, lease); err == nil {
		return landedHead, nil
	}
	out, err := git(root, "cherry", landedHead, lease)
	if err != nil {
		return "", err
	}
	boundary := ""
	for _, line := range strings.Split(out, "\n") {
		if sha, ok := strings.CutPrefix(line, "- "); ok {
			boundary = sha
		}
	}
	return boundary, nil
}

func filePatches(root, from, to string) (map[string]string, error) {
	files, err := names(root, from, to)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range files {
		if out[f], err = patchID(root, from+".."+to, f); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func differing(a, b map[string]string) []string {
	var out []string
	for f, id := range a {
		if b[f] != id {
			out = append(out, f)
		}
	}
	for f := range b {
		if _, ok := a[f]; !ok {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return out
}

func PushRepair(e Env, dependent int) (Repair, error) {
	root, err := git(e.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repair{}, errors.New("not inside a git repository")
	}
	file, err := repairFile(root, dependent)
	if err != nil {
		return Repair{}, err
	}
	raw, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return Repair{}, fmt.Errorf("no repair of #%d is waiting; run harness land repair %d --landed <pr> first", dependent, dependent)
	}
	if err != nil {
		return Repair{}, err
	}
	var r Repair
	if err := json.Unmarshal(raw, &r); err != nil {
		return Repair{}, fmt.Errorf("%s: %w", file, err)
	}
	if head, err := git(r.Worktree, "rev-parse", "HEAD"); err != nil || head != r.Rebased {
		return r, fmt.Errorf("%s is no longer at the rebased head %s; repair again", r.Worktree, short(r.Rebased))
	}
	_, pushErr := git(root, "push", "--quiet", "--force-with-lease=refs/heads/"+r.Branch+":"+r.Lease, "origin", r.Rebased+":refs/heads/"+r.Branch)
	if r.Temporary {
		if _, err := git(root, "worktree", "remove", "--force", r.Worktree); err != nil {
			return r, err
		}
		os.RemoveAll(filepath.Dir(r.Worktree))
	}
	if err := os.Remove(file); err != nil {
		return r, err
	}
	if pushErr != nil {
		return r, fmt.Errorf("the push was refused, so origin keeps its %s; someone may have pushed since the repair read it: %w", r.Branch, pushErr)
	}
	return r, nil
}
