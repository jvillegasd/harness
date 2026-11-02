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
)

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
	if err := run(scratch, stdout, stderr, "npx", "skills@1", "add", source, "--skill", name, "--agent", "universal", "-y"); err != nil {
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

// update runs skills.sh against a scratch copy of the lock, which is all it needs to compare every entry with upstream, then copies each body it rewrote into the group that already holds it.
func update(c checkout.Checkout, stdout, stderr io.Writer) error {
	p := pathsFor(c)
	scratch, err := scratchProject(p)
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	if err := run(scratch, stdout, stderr, "npx", "skills@1", "update", "--project", "--yes"); err != nil {
		return err
	}
	updated, err := os.ReadDir(filepath.Join(scratch, ".agents", "skills"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
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
		if err := os.RemoveAll(body); err != nil {
			return err
		}
		if err := copyTree(filepath.Join(scratch, ".agents", "skills", e.Name()), body); err != nil {
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

// checkLink refuses to remove anything at path but a symlink to want; an empty want accepts any target, for a link whose body is already gone.
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

// findBody returns skills/<group>/<name> from whichever group holds it, or "" when none does. A file or symlink at that path is refused, because removing or replacing it could reach outside the checkout.
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

// linkSkill points .agents/skills/<name> at the grouped body and .claude/skills/<name> at that link, so every client finds the working-tree skill by its bare name.
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

// scratchProject is a throwaway directory holding a copy of the lock, where skills.sh can run without writing into the checkout's grouped tree.
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

// copyTree copies the regular files and directories under src to dst, keeping each file's mode.
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
		fmt.Fprint(stdout, "\nLocked-skill changes:\n")
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
	fmt.Fprint(stdout, "\nReview the upstream body, then commit only the approved change\n")
	return nil
}

func insideWorkTree(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}
