// Package checkout locates the nearest enclosing Harness checkout from the invocation directory and names its canonical asset locations, so every command resolves that checkout and its layout the same way.
package checkout

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// module is the Go module path that identifies a Harness checkout, so a directory holding a file at the root floor marker's path is not mistaken for this checkout.
const module = "github.com/jvillegasd/harness"

// floorMarker is the checkout-relative root floor file that marks a Harness checkout: the product assets live at the repository root, so the marker sits beside the module.
const floorMarker = "FLOOR.md"

// ErrNotFound names the requirement when no Harness checkout encloses the invocation directory.
var ErrNotFound = errors.New("run harness from inside a Harness checkout")

// Checkout is the root of a Harness checkout.
type Checkout struct {
	Root string
}

// Find returns the checkout whose root floor marker is nearest at or above dir.
//
// A Harness checkout is the root floor marker together with the Harness Go module, so the first directory holding the marker decides the answer: it is returned when it declares module github.com/jvillegasd/harness, and a marker with no module or beside a different one stops the walk with ErrNotFound rather than resolving an enclosing checkout. A partial tree or a different module is therefore never silently substituted. The walk starts at dir and stops at the filesystem root, so it never falls back to an installed binary's checkout or a historical location.
func Find(dir string) (Checkout, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Checkout{}, err
	}
	for {
		_, err := os.Stat(filepath.Join(abs, floorMarker))
		if err == nil {
			return findRoot(abs)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Checkout{}, err
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return Checkout{}, ErrNotFound
		}
		abs = parent
	}
}

// findRoot returns the checkout marked at dir when dir declares the Harness module, and otherwise the error that stops the walk there.
func findRoot(dir string) (Checkout, error) {
	mod, err := readModule(dir)
	if err != nil {
		return Checkout{}, err
	}
	switch mod {
	case module:
		return Checkout{Root: dir}, nil
	case "":
		return Checkout{}, fmt.Errorf("%w: no Harness module at %s", ErrNotFound, dir)
	default:
		return Checkout{}, fmt.Errorf("%w: %s declares module %s, not the Harness module", ErrNotFound, dir, mod)
	}
}

// readModule returns the module path declared at dir, or "" when dir declares no module.
func readModule(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return moduleOf(data), nil
}

// moduleOf returns the module path a go.mod declares, or "" when it declares none.
func moduleOf(gomod []byte) string {
	for _, line := range strings.Split(string(gomod), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

// Floor is the root floor marker, which installs as each client's global instruction file.
func (c Checkout) Floor() string { return filepath.Join(c.Root, floorMarker) }

// Skills is the directory of harness skills.
func (c Checkout) Skills() string { return filepath.Join(c.Root, "skills") }

// Skill is one skill in the checkout: its name, which clients see flat, and its grouped directory.
type Skill struct {
	Name string
	Dir  string
}

// SkillDirs lists every skill as skills/<group>/<name>, sorted by name. Groups only organize the tree: clients and the lock see the bare name, so a name used in two groups is an error, and so is a skill placed directly under skills/. Every folder inside a group is listed, so a caller can report one that lacks its SKILL.md. Hidden entries, files, and symlinks are skipped.
func (c Checkout) SkillDirs() ([]Skill, error) {
	groups, err := os.ReadDir(c.Skills())
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(c.Skills(), "SKILL.md")); err == nil {
		return nil, fmt.Errorf("%s holds a SKILL.md; a skill belongs in skills/<group>/<name>", c.Skills())
	}
	seen := map[string]string{}
	var out []Skill
	for _, group := range groups {
		if strings.HasPrefix(group.Name(), ".") || !group.IsDir() {
			continue
		}
		groupDir := filepath.Join(c.Skills(), group.Name())
		if _, err := os.Stat(filepath.Join(groupDir, "SKILL.md")); err == nil {
			return nil, fmt.Errorf("skill %s sits directly under skills/; move it into a group folder", group.Name())
		}
		list, err := os.ReadDir(groupDir)
		if err != nil {
			return nil, err
		}
		for _, entry := range list {
			if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() {
				continue
			}
			dir := filepath.Join(groupDir, entry.Name())
			if prev, ok := seen[entry.Name()]; ok {
				return nil, fmt.Errorf("skill %s is in two groups: %s and %s", entry.Name(), prev, dir)
			}
			seen[entry.Name()] = dir
			out = append(out, Skill{Name: entry.Name(), Dir: dir})
		}
	}
	slices.SortFunc(out, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// SkillDir returns the grouped directory of the named skill, and false when no group holds it.
func (c Checkout) SkillDir(name string) (string, bool, error) {
	skills, err := c.SkillDirs()
	if err != nil {
		return "", false, err
	}
	for _, s := range skills {
		if s.Name == name {
			return s.Dir, true, nil
		}
	}
	return "", false, nil
}

// Aliases is the shell file holding the managed yolo aliases.
func (c Checkout) Aliases() string { return filepath.Join(c.Root, "aliases.sh") }

// Clients is the directory of client templates and role assignments.
func (c Checkout) Clients() string { return filepath.Join(c.Root, "clients") }

// Generated is the untracked directory of rendered role agents.
func (c Checkout) Generated() string { return filepath.Join(c.Root, ".generated") }

// Bench is the directory of benchmark cohorts and assets.
func (c Checkout) Bench() string { return filepath.Join(c.Root, "bench") }

// Lock is the reviewed third-party skill lock at the checkout root.
func (c Checkout) Lock() string { return filepath.Join(c.Root, "skills-lock.json") }

// AgentSkills is the checkout's project skill link directory, one link per skill into its group, which Codex and OpenCode read and the Claude links point through.
func (c Checkout) AgentSkills() string { return filepath.Join(c.Root, ".agents", "skills") }

// ClaudeSkills is the checkout's Claude skill link directory.
func (c Checkout) ClaudeSkills() string { return filepath.Join(c.Root, ".claude", "skills") }

// Managed names the assets the installer owns as link sources. Ownership is these namespaces alone, never everything beneath the checkout, so promoting the assets to the repository root does not make an unrelated repository file, a private skill, or private data installer-owned.
func (c Checkout) Managed() []string {
	return []string{c.Skills(), c.Floor(), c.Generated()}
}

// Manages reports whether path is a managed asset or sits beneath one, which is the only predicate an install or uninstall may use to claim a link.
func (c Checkout) Manages(path string) bool {
	for _, root := range c.Managed() {
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}
