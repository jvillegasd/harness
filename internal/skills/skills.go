package skills

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/ui"
)

const skillsCLI = "skills@1.8.1"

var (
	nameRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	ownerRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$`)
	repoRE  = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.-]*$`)
)

func validateName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid skill name: %s", name)
	}
	return nil
}

func validateSource(source string) error {
	owner, repo, ok := strings.Cut(source, "/")
	if !ok {
		return fmt.Errorf("source must be owner/repo: %s", source)
	}
	if !ownerRE.MatchString(owner) {
		return fmt.Errorf("invalid GitHub owner: %s", owner)
	}
	if !repoRE.MatchString(repo) || repo == "." || repo == ".." {
		return fmt.Errorf("invalid GitHub repository: %s", repo)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func run(root string, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func add(c checkout.Checkout, args []string, stdout, stderr io.Writer) error {
	source, name, group := args[0], args[1], args[2]
	p := pathsFor(c)
	lf, err := readLock(p.lock)
	if err != nil {
		return err
	}
	if lf.skills.has(name) {
		return fmt.Errorf("%s is already locked; use update", name)
	}
	if body, err := findBody(p, name); err != nil {
		return err
	} else if body != "" {
		return fmt.Errorf("skill path already exists: %s", body)
	}
	body := filepath.Join(p.skills, group, name)
	agentLink := filepath.Join(p.agents, name)
	link := filepath.Join(p.claude, name)
	for _, path := range []string{agentLink, link} {
		if exists(path) {
			return fmt.Errorf("claude path already exists: %s", path)
		}
	}
	scratch, err := scratchProject(p)
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	if err := run(scratch, stdout, stderr, "npx", skillsCLI, "add", source, "--skill", name, "--agent", "universal", "-y"); err != nil {
		return err
	}
	installed := filepath.Join(scratch, ".agents", "skills", name)
	if fi, err := os.Stat(filepath.Join(installed, "SKILL.md")); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("skills.sh did not install %s", name)
	}
	scratchLock, err := readLock(filepath.Join(scratch, "skills-lock.json"))
	if err != nil {
		return err
	}
	if !scratchLock.skills.has(name) {
		return fmt.Errorf("skills.sh did not lock %s", name)
	}
	if err := copyTree(installed, body); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(scratch, "skills-lock.json"), p.lock); err != nil {
		return err
	}
	if err := linkSkill(p, name, body); err != nil {
		return err
	}
	return reviewPaths(p, stdout)
}

func update(c checkout.Checkout, stdout, stderr io.Writer) error {
	p := pathsFor(c)
	if err := CheckUpdate(c); err != nil {
		return err
	}
	scratch, err := scratchProject(p)
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	if err := run(scratch, stdout, stderr, "npx", skillsCLI, "update", "--project", "--yes"); err != nil {
		return err
	}
	updated, err := os.ReadDir(filepath.Join(scratch, ".agents", "skills"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	type swap struct{ body, staged string }
	var swaps []swap
	defer func() {
		for _, sw := range swaps {
			os.RemoveAll(sw.staged)
		}
	}()
	for _, e := range updated {
		if !e.IsDir() {
			continue
		}
		body, err := findBody(p, e.Name())
		if err != nil {
			return err
		}
		if body == "" {
			return fmt.Errorf("skills.sh updated %s, which no skill group holds", e.Name())
		}
		staged, err := os.MkdirTemp(filepath.Dir(body), ".harness-update-*")
		if err != nil {
			return err
		}
		swaps = append(swaps, swap{body, staged})
		if err := copyTree(filepath.Join(scratch, ".agents", "skills", e.Name()), staged); err != nil {
			return err
		}
	}
	if err := CheckUpdate(c); err != nil {
		return err
	}
	for _, sw := range swaps {
		if err := os.RemoveAll(sw.body); err != nil {
			return err
		}
		if err := os.Rename(sw.staged, sw.body); err != nil {
			return err
		}
	}
	if err := copyFile(filepath.Join(scratch, "skills-lock.json"), p.lock); err != nil {
		return err
	}
	return reviewPaths(p, stdout)
}

func remove(c checkout.Checkout, args []string, stdout, stderr io.Writer) error {
	name := args[0]
	p := pathsFor(c)
	lf, err := readLock(p.lock)
	if err != nil {
		return err
	}
	if !lf.skills.has(name) {
		return fmt.Errorf("%s is not in skills-lock.json", name)
	}
	body, err := findBody(p, name)
	if err != nil {
		return err
	}
	agentLink := filepath.Join(p.agents, name)
	link := filepath.Join(p.claude, name)
	agentWant := ""
	if body != "" {
		if agentWant, err = filepath.Rel(p.agents, body); err != nil {
			return err
		}
	}
	if err := checkLink(agentLink, agentWant); err != nil {
		return err
	}
	if err := checkLink(link, "../../.agents/skills/"+name); err != nil {
		return err
	}
	lf.skills.del(name)
	if err := writeLock(p.lock, lf); err != nil {
		return err
	}
	if body != "" {
		if err := os.RemoveAll(body); err != nil {
			return err
		}
	}
	for _, path := range []string{agentLink, link} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return reviewPaths(p, stdout)
}

func checkLink(path, want string) error {
	if !exists(path) {
		return nil
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("refusing to remove non-symlink Claude path: %s", path)
	}
	target, err := os.Readlink(path)
	if err != nil {
		return err
	}
	if want != "" && target != want {
		return fmt.Errorf("refusing to remove unexpected Claude link: %s", path)
	}
	return nil
}

func findBody(p paths, name string) (string, error) {
	groups, err := os.ReadDir(p.skills)
	if err != nil {
		return "", err
	}
	found := ""
	for _, g := range groups {
		if !g.IsDir() || strings.HasPrefix(g.Name(), ".") {
			continue
		}
		candidate := filepath.Join(p.skills, g.Name(), name)
		fi, err := os.Lstat(candidate)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing to remove non-directory skill path: %s", candidate)
		}
		if found != "" {
			return "", fmt.Errorf("skill %s is in two groups: %s and %s", name, found, candidate)
		}
		found = candidate
	}
	return found, nil
}

func linkSkill(p paths, name, body string) error {
	for _, dir := range []string{p.agents, p.claude} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	rel, err := filepath.Rel(p.agents, body)
	if err != nil {
		return err
	}
	if err := os.Symlink(rel, filepath.Join(p.agents, name)); err != nil {
		return err
	}
	return os.Symlink("../../.agents/skills/"+name, filepath.Join(p.claude, name))
}

func scratchProject(p paths) (string, error) {
	dir, err := os.MkdirTemp("", "harness-skills-")
	if err != nil {
		return "", err
	}
	if err := copyFile(p.lock, filepath.Join(dir, "skills-lock.json")); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func reviewPaths(p paths, stdout io.Writer) error {
	if insideWorkTree(p.root) {
		fmt.Fprintln(stdout, "\n"+ui.For(stdout).Title("harness skills", "locked-skill changes"))
		within := func(abs string) string {
			rel, err := filepath.Rel(p.root, abs)
			if err != nil {
				return abs
			}
			return rel
		}
		cmd := exec.Command("git", "-C", p.root, "status", "--short", "--", within(p.skills), within(p.lock), within(p.agents), within(p.claude))
		cmd.Stdout = stdout
		cmd.Stderr = io.Discard
		_ = cmd.Run()
	}
	fmt.Fprintln(stdout, "\n"+ui.For(stdout).Warn("Review the upstream body and lockfile before running harness install; commit only the approved change"))
	return nil
}

func insideWorkTree(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}

func CheckUpdate(c checkout.Checkout) error {
	p := pathsFor(c)
	lf, err := readLock(p.lock)
	if err != nil {
		return err
	}
	paths := []string{"skills-lock.json"}
	for _, name := range lf.skills.keys {
		if err := validateName(name); err != nil {
			return err
		}
		body, err := findBody(p, name)
		if err != nil {
			return err
		}
		if body == "" {
			return fmt.Errorf("locked body missing: %s", name)
		}
		rel, err := filepath.Rel(p.root, body)
		if err != nil {
			return err
		}
		paths = append(paths, rel)
	}
	args := append([]string{"-C", p.root, "--literal-pathspecs", "status", "--porcelain=v1", "--untracked-files=all", "--ignored", "--"}, paths...)
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("checking locked paths requires a Git checkout: %w: %s", err, out)
	}
	if len(out) > 0 {
		return fmt.Errorf("local changes in locked paths; commit or preserve them before updating:\n%s", out)
	}
	return nil
}
