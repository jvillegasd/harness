package update

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type formula struct {
	Name      string `json:"name"`
	FullName  string `json:"full_name"`
	Pinned    bool   `json:"pinned"`
	Installed []struct {
		Version string `json:"version"`
	} `json:"installed"`
}
type cask struct {
	Token     string                       `json:"token"`
	FullToken string                       `json:"full_token"`
	Installed json.RawMessage              `json:"installed"`
	Artifacts []map[string]json.RawMessage `json:"artifacts"`
}
type inventory struct {
	Formulae     []formula `json:"formulae"`
	Casks        []cask    `json:"casks"`
	prefix, brew string
	noQuit       bool
}

func (r *runner) inventory() inventory {
	var inv inventory
	brew, err := exec.LookPath("brew")
	if err != nil {
		return inv
	}
	inv.brew = brew
	out, err := exec.Command(brew, "info", "--json=v2", "--installed").Output()
	if err == nil {
		err = json.Unmarshal(out, &inv)
	}
	if err != nil {
		r.report("failed", "Homebrew inventory", err.Error())
		return inventory{}
	}
	out, err = exec.Command(brew, "--prefix").Output()
	if err != nil || !filepath.IsAbs(strings.TrimSpace(string(out))) {
		r.report("failed", "Homebrew prefix", "cannot establish package ownership")
		return inventory{}
	}
	inv.prefix = resolved(strings.TrimSpace(string(out)))
	if len(inv.Casks) > 0 {
		help, err := exec.Command(brew, "upgrade", "--help").Output()
		inv.noQuit = err == nil && strings.Contains(string(help), "--no-quit")
	}
	return inv
}
func within(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}
func resolved(path string) string {
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return p
}
func orcaManaged(path string) bool {
	p := strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(p, "/orca/") || strings.Contains(p, "/orca.app/") || strings.Contains(p, "/orca preview.app/") || strings.Contains(p, "/orca-dev/")
}
func (inv inventory) brewTarget(group, name, path string) (target, bool) {
	t := target{group: group, name: name, path: path}
	matches := 0
	for _, f := range inv.Formulae {
		if (f.Name != name && !strings.HasPrefix(f.Name, name+"@")) || len(f.Installed) == 0 || !within(resolved(path), filepath.Join(inv.prefix, "Cellar", f.Name)) {
			continue
		}
		matches++
		token := f.FullName
		if token == "" {
			token = f.Name
		}
		t.command = []string{inv.brew, "upgrade", "--formula", token}

		if f.Pinned {
			t.status = "manual"
			t.notice = "Homebrew formula is pinned; update manually without changing the pin"
		}
	}
	for _, c := range inv.Casks {
		if c.Token != name && !(name == "claude" && c.Token == "claude-code") {
			continue
		}
		if !c.installed() {
			continue
		}
		owned := within(resolved(path), filepath.Join(inv.prefix, "Caskroom", c.Token))
		for _, artifact := range c.Artifacts {
			var apps []string
			if json.Unmarshal(artifact["app"], &apps) == nil {
				for _, app := range apps {
					if within(resolved(path), filepath.Join("/Applications", app)) || within(resolved(path), filepath.Join(os.Getenv("HOME"), "Applications", app)) {
						owned = true
					}
				}
			}
		}
		if !owned {
			continue
		}
		matches++
		t.command = []string{inv.brew, "upgrade", "--cask", "--no-quit", c.token()}
		if !inv.noQuit {
			t.status = "manual"
			t.notice = "Homebrew lacks --no-quit; update this cask manually"
		}
	}
	if matches > 1 {
		t.status = "manual"
		t.notice = "ambiguous Homebrew ownership"
	}
	return t, matches > 0
}
func (c cask) token() string {
	if c.FullToken != "" {
		return c.FullToken
	}
	return c.Token
}
func (c cask) installed() bool {
	return len(c.Installed) > 0 && string(c.Installed) != "null" && string(c.Installed) != "[]" && string(c.Installed) != `""`
}

func (r *runner) discover(selected []string) []target {
	var inv inventory
	if len(selected) != 1 || selected[0] != "skills" {
		inv = r.inventory()
	}
	if slices.Contains(selected, "mcps") {
		for _, client := range []string{"claude", "codex", "opencode"} {
			if _, err := exec.LookPath(client); err == nil {
				r.clients = append(r.clients, client)
			}
		}
	}
	var targets []target
	for _, group := range selected {
		names := map[string][]string{"packages": {"node", "gh", "uv"}, "clients": {"claude", "codex", "opencode"}, "mcps": {"engram", "agent-browser"}}[group]
		for _, name := range names {
			t := target{group: group, name: name, status: "skipped", notice: "not installed"}
			path, err := exec.LookPath(name)
			if err == nil {
				t.path = path
				t.status = "manual"
				t.notice = "unrecognized installation; update through its owner"
				switch {
				case group == "clients" && orcaManaged(resolved(path)):
					t.status = "skipped"
					t.notice = "owned by Orca; update Orca instead"
				default:
					if owned, ok := inv.brewTarget(group, name, path); ok {
						t = owned
					} else if group == "clients" {
						t = r.nativeClient(t)
					}
				}
			}
			targets = append(targets, t)
		}
	}
	// Deduplicate the complete plan.
	seen := map[string]bool{}
	for i := range targets {
		t := &targets[i]
		if len(t.command) == 0 {
			continue
		}
		key := strings.Join(t.command, "\x00")
		if seen[key] {
			t.status = "skipped"
			t.notice = "already scheduled"
		}
		seen[key] = true
	}
	return targets
}

func (r *runner) nativeClient(t target) target {
	home := resolved(os.Getenv("HOME"))
	path := resolved(t.path)
	switch t.name {
	case "claude":
		if filepath.IsAbs(home) && within(path, filepath.Join(home, ".local", "share", "claude", "versions")) {
			t.status = ""
			t.command = []string{t.path, "update"}
		}
	case "opencode":
		if filepath.IsAbs(home) && path == filepath.Join(home, ".opencode", "bin", "opencode") {
			v := version(t.path)
			if v != "" && !strings.ContainsAny(v, "-+") {
				t.status = ""
				t.command = []string{t.path, "upgrade", "--method", "curl"}
			} else {
				t.notice = "native OpenCode release channel unclear; use opencode upgrade manually"
			}
		}
	case "codex":
		// npm's symlink resolves into <prefix>/lib/node_modules/@openai/codex.
		marker := string(filepath.Separator) + filepath.Join("lib", "node_modules", "@openai", "codex") + string(filepath.Separator)
		prefix, _, ok := strings.Cut(path, marker)
		if !ok || !filepath.IsAbs(prefix) {
			break
		}
		npm, err := exec.LookPath("npm")
		if err != nil {
			break
		}
		out, err := exec.Command(npm, "ls", "--global", "--prefix", prefix, "--json", "--depth=0").Output()
		var record struct {
			Dependencies map[string]struct {
				Version string `json:"version"`
			} `json:"dependencies"`
		}
		if err != nil || json.Unmarshal(out, &record) != nil {
			break
		}
		v := record.Dependencies["@openai/codex"].Version
		if v == "" || strings.ContainsAny(v, "-+") {
			t.notice = "npm release channel unclear; update @openai/codex manually with its original tag"
			break
		}
		t.status = ""
		t.command = []string{npm, "install", "--global", "--prefix", prefix, "@openai/codex@latest"}
	}
	return t
}
