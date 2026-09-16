package locks

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
	if len(args) != 2 {
		return errors.New("add expects 2 argument(s)")
	}
	source, name := args[0], args[1]
	if err := validateSource(source); err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return err
	}
	p := pathsFor(c)
	lf, err := readLock(p.lock)
	if err != nil {
		return err
	}
	if lf.skills.has(name) {
		return fmt.Errorf("%s is already locked; use update", name)
	}
	body := filepath.Join(p.skills, name)
	command := filepath.Join(p.commands, name+".md")
	link := filepath.Join(p.claude, name)
	if exists(body) {
		return fmt.Errorf("skill path already exists: %s", body)
	}
	if exists(command) {
		return fmt.Errorf("command name already exists: %s", command)
	}
	if exists(link) {
		return fmt.Errorf("claude path already exists: %s", link)
	}
	if err := run(c.Root, stdout, stderr, "npx", "skills@1", "add", source, "--skill", name, "--agent", "universal", "-y"); err != nil {
		return err
	}
	if fi, err := os.Stat(filepath.Join(body, "SKILL.md")); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("skills.sh did not install %s", name)
	}
	lf, err = readLock(p.lock)
	if err != nil {
		return err
	}
	if !lf.skills.has(name) {
		return fmt.Errorf("skills.sh did not lock %s", name)
	}
	if err := os.MkdirAll(p.claude, 0o755); err != nil {
		return err
	}
	if err := os.Symlink("../../.agents/skills/"+name, link); err != nil {
		return err
	}
	return reviewPaths(p, stdout)
}

func update(c checkout.Checkout, args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return errors.New("update expects 0 argument(s)")
	}
	if err := run(c.Root, stdout, stderr, "npx", "skills@1", "update", "--project", "--yes"); err != nil {
		return err
	}
	return reviewPaths(pathsFor(c), stdout)
}

func remove(c checkout.Checkout, args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return errors.New("remove expects 1 argument(s)")
	}
	name := args[0]
	if err := validateName(name); err != nil {
		return err
	}
	p := pathsFor(c)
	lf, err := readLock(p.lock)
	if err != nil {
		return err
	}
	if !lf.skills.has(name) {
		return fmt.Errorf("%s is not in skills-lock.json", name)
	}
	body := filepath.Join(p.skills, name)
	link := filepath.Join(p.claude, name)
	if exists(body) {
		fi, err := os.Lstat(body)
		if err != nil {
			return err
		}
		if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to remove non-directory skill path: %s", body)
		}
	}
	if exists(link) {
		fi, err := os.Lstat(link)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refusing to remove non-symlink Claude path: %s", link)
		}
		target, err := os.Readlink(link)
		if err != nil {
			return err
		}
		if target != "../../.agents/skills/"+name {
			return fmt.Errorf("refusing to remove unexpected Claude link: %s", link)
		}
	}
	lf.skills.del(name)
	if err := writeLock(p.lock, lf); err != nil {
		return err
	}
	if err := os.RemoveAll(body); err != nil {
		return err
	}
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return reviewPaths(p, stdout)
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
		cmd := exec.Command("git", "-C", p.root, "status", "--short", "--", within(p.skills), within(p.lock), within(p.claude))
		cmd.Stdout = stdout
		cmd.Stderr = io.Discard
		_ = cmd.Run()
	}
	fmt.Fprint(stdout, "\nReview the upstream body and run: harness locks check\n")
	return nil
}

func insideWorkTree(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}
