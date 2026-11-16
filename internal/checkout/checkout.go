package checkout

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const module = "github.com/jvillegasd/harness"

const floorMarker = "FLOOR.md"

var ErrNotFound = errors.New("run harness from inside a Harness checkout")

type Checkout struct {
	Root string
}

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

func moduleOf(gomod []byte) string {
	for _, line := range strings.Split(string(gomod), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

func (c Checkout) Floor() string { return filepath.Join(c.Root, floorMarker) }

func (c Checkout) Skills() string { return filepath.Join(c.Root, "skills") }

// EntryGroup is the one group whose skills a client discovers by name; every
// other group installs where only a path reaches it.
const EntryGroup = "workflow"

type Skill struct {
	Name  string
	Dir   string
	Group string
}

// Shown names the skills a client discovers although they sit outside
// EntryGroup: a skill a playbook calls that the human also drives by name.
var Shown = map[string]bool{"impeccable": true}

// Entry reports whether a client discovers the skill by name.
func (s Skill) Entry() bool { return s.Group == EntryGroup || Shown[s.Name] }

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
			out = append(out, Skill{Name: entry.Name(), Dir: dir, Group: group.Name()})
		}
	}
	slices.SortFunc(out, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (c Checkout) Aliases() string { return filepath.Join(c.Root, "aliases.sh") }

func (c Checkout) Clients() string { return filepath.Join(c.Root, "clients") }

func (c Checkout) Generated() string { return filepath.Join(c.Root, ".generated") }

func (c Checkout) Bench() string { return filepath.Join(c.Root, "bench") }

func (c Checkout) Lock() string { return filepath.Join(c.Root, "skills-lock.json") }

func (c Checkout) AgentSkills() string { return filepath.Join(c.Root, ".agents", "skills") }

func (c Checkout) ClaudeSkills() string { return filepath.Join(c.Root, ".claude", "skills") }
