package skills

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jvillegasd/harness/internal/checkout"
)

// group returns skills/<group> when it is an existing group directory.
func group(p paths, name string) (string, error) {
	dir := filepath.Join(p.skills, name)
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("no skill group %s under %s", name, p.skills)
	}
	return dir, nil
}

// starter is the body `new` writes: frontmatter naming the skill, with every guard when guarded, and a heading to write under.
func starter(name string, guarded bool) string {
	var b strings.Builder
	b.WriteString("---\nname: " + name + "\ndescription: \"Say what this skill does and when to use it.\"\n")
	if guarded {
		b.WriteString("disable-model-invocation: true\nmetadata:\n  opencode/autoinvoke: \"false\"\n")
	}
	b.WriteString("---\n\n# " + name + "\n")
	return b.String()
}

// New creates a local skill in an existing group with a starting body and both bookkeeping links. With guarded it carries all three invocation guards, one per client, and otherwise none. It refuses a name any group or the lock already holds, and writes nothing then.
func New(c checkout.Checkout, name, groupName string, guarded bool) ([]string, error) {
	p := pathsFor(c)
	if err := validateName(name); err != nil {
		return nil, err
	}
	dir, err := group(p, groupName)
	if err != nil {
		return nil, err
	}
	if body, err := findBody(p, name); err != nil {
		return nil, err
	} else if body != "" {
		return nil, fmt.Errorf("skill %s already exists: %s", name, body)
	}
	if lf, err := readLock(p.lock); err != nil {
		return nil, err
	} else if lf.skills.has(name) {
		return nil, fmt.Errorf("skill %s is already locked", name)
	}
	for _, link := range []string{filepath.Join(p.agents, name), filepath.Join(p.claude, name)} {
		if exists(link) {
			return nil, fmt.Errorf("skill link already exists: %s", link)
		}
	}
	body := filepath.Join(dir, name)
	files := map[string]string{filepath.Join(body, "SKILL.md"): starter(name, guarded)}
	if guarded {
		files[filepath.Join(body, "agents", "openai.yaml")] = "policy:\n  allow_implicit_invocation: false\n"
	}
	for path, text := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return nil, err
		}
	}
	if err := linkSkill(p, name, body); err != nil {
		return nil, err
	}
	return []string{"created " + rel(p, body), "linked " + rel(p, filepath.Join(p.agents, name)), "linked " + rel(p, filepath.Join(p.claude, name))}, nil
}

// Move moves a skill's body to another existing group with git mv where Git tracks it, so its history follows, and repoints its .agents/skills link. The body's content is unchanged, so a locked body keeps matching its lock. It refuses an unknown skill or group, and the group the skill is already in, changing nothing.
func Move(c checkout.Checkout, name, groupName string) ([]string, error) {
	p := pathsFor(c)
	if err := validateName(name); err != nil {
		return nil, err
	}
	body, err := findBody(p, name)
	if err != nil {
		return nil, err
	}
	if body == "" {
		return nil, fmt.Errorf("no skill %s in any group", name)
	}
	dir, err := group(p, groupName)
	if err != nil {
		return nil, err
	}
	if filepath.Dir(body) == dir {
		return nil, fmt.Errorf("skill %s is already in %s", name, groupName)
	}
	agentLink := filepath.Join(p.agents, name)
	oldRel, err := filepath.Rel(p.agents, body)
	if err != nil {
		return nil, err
	}
	if err := checkLink(agentLink, oldRel); err != nil {
		return nil, err
	}
	link := filepath.Join(p.claude, name)
	if err := checkLink(link, "../../.agents/skills/"+name); err != nil {
		return nil, err
	}
	target := filepath.Join(dir, name)
	if tracked(p.root, body) {
		cmd := exec.Command("git", "-C", p.root, "mv", "--", rel(p, body), rel(p, target))
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git mv: %w: %s", err, strings.TrimSpace(string(out)))
		}
	} else if err := os.Rename(body, target); err != nil {
		return nil, err
	}
	if err := os.Remove(agentLink); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if exists(link) {
		if err := os.Remove(link); err != nil {
			return nil, err
		}
	}
	if err := linkSkill(p, name, target); err != nil {
		return nil, err
	}
	return []string{"moved " + rel(p, body) + " to " + rel(p, target), "linked " + rel(p, agentLink)}, nil
}

// tracked reports whether Git tracks any file under path.
func tracked(root, path string) bool {
	out, err := exec.Command("git", "-C", root, "ls-files", "--", path).Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func rel(p paths, path string) string {
	if r, err := filepath.Rel(p.root, path); err == nil {
		return r
	}
	return path
}

func newSkill(c checkout.Checkout, args []string, stdout io.Writer) error {
	guarded := false
	var rest []string
	for _, a := range args {
		if a == "--guarded" {
			guarded = true
			continue
		}
		rest = append(rest, a)
	}
	done, err := New(c, rest[0], rest[1], guarded)
	for _, line := range done {
		fmt.Fprintln(stdout, line)
	}
	return err
}

func moveSkill(c checkout.Checkout, args []string, stdout io.Writer) error {
	done, err := Move(c, args[0], args[1])
	for _, line := range done {
		fmt.Fprintln(stdout, line)
	}
	return err
}
