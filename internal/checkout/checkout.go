// Package checkout locates the nearest enclosing Harness checkout from the invocation directory and names its canonical asset locations, so every command resolves that checkout and its layout the same way.
package checkout

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// module is the Go module path that identifies a Harness checkout, so a directory holding a file at the root policy marker's path is not mistaken for this checkout.
const module = "github.com/jvillegasd/harness"

// policyMarker is the checkout-relative root policy file that marks a Harness checkout: the product assets live at the repository root, so the marker sits beside the module.
const policyMarker = "POLICY.md"

// ErrNotFound names the requirement when no Harness checkout encloses the invocation directory.
var ErrNotFound = errors.New("run harness from inside a Harness checkout")

// Checkout is the root of a Harness checkout.
type Checkout struct {
	Root string
}

// Find returns the checkout whose root policy marker is nearest at or above dir.
//
// A Harness checkout is the root policy marker together with the Harness Go module, so the first directory holding the marker decides the answer: it is returned when it declares module github.com/jvillegasd/harness, and a marker with no module or beside a different one stops the walk with ErrNotFound rather than resolving an enclosing checkout. A partial tree or a different module is therefore never silently substituted. The walk starts at dir and stops at the filesystem root, so it never falls back to an installed binary's checkout or a historical location.
func Find(dir string) (Checkout, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Checkout{}, err
	}
	for {
		_, err := os.Stat(filepath.Join(abs, policyMarker))
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

// Policy is the root policy marker that links into each client's global instruction path.
func (c Checkout) Policy() string { return filepath.Join(c.Root, policyMarker) }

// Skills is the directory of harness skills.
func (c Checkout) Skills() string { return filepath.Join(c.Root, "skills") }

// Commands is the directory of harness commands.
func (c Checkout) Commands() string { return filepath.Join(c.Root, "commands") }

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

// ClaudeSkills is the checkout's Claude skill link directory.
func (c Checkout) ClaudeSkills() string { return filepath.Join(c.Root, ".claude", "skills") }

// Managed names the assets the installer owns as link sources. Ownership is these namespaces alone, never everything beneath the checkout, so promoting the assets to the repository root does not make an unrelated repository file, a private skill, or private data installer-owned.
func (c Checkout) Managed() []string {
	return []string{c.Skills(), c.Commands(), c.Policy(), c.Generated()}
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
