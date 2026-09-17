package install

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/statusline"
)

// The two MCP server id prefixes engram registers. A permission entry is engram's when it starts with one, never when it merely mentions the word.
var engramToolPrefixes = []string{"mcp__engram__", "mcp__plugin_engram_engram__"}

// subagentStatusline is the OpenCode built-in `engram setup opencode` enables, the one entry it writes that is not named after engram.
const subagentStatusline = "opencode-subagent-statusline"

var yoloAlias = regexp.MustCompile(`(?m)^[ \t]*alias (ccyolo|oyolo|ocyolo)=`)

const uninstallGone = `Gone: every managed skill and command link, both POLICY.md links, the role
agent links and their generated definitions, the ~/.zshrc alias block, the
tracked keys in ~/.claude/settings.json, Engram: its plugin, marketplace, and
MCP registration in both clients, the OpenCode subagent status line it enables,
the formula, and ~/.engram, agent-browser: its MCP registration in both
clients, the formula, and ~/.agent-browser, every archive under
~/.dotfiles-backups/harness, the status line cache, the client directories the
harness left empty, and ~/.local/bin/harness.
`

const uninstallLeft = `
Left alone: every untracked key in the Claude config, the rest of
~/.dotfiles-backups, Worktrunk's shell integration, and this checkout.

Nothing was archived: an uninstall deletes what it removes.

Open a new shell to drop the aliases from the one you are in.
`

// sharedPackages are what a full install adds when missing beside Engram and agent-browser. The install never records whether it added one or found it, so each goes only on the user's say-so.
var sharedPackages = []string{"node", "gh", "uv", "worktrunk", "anomalyco/tap/opencode"}

func (in *installer) uninstallAll() {
	in.log("Removing the harness from this machine")
	engramKept := in.uninstallEngram()
	agentBrowserKept := in.uninstallAgentBrowser()
	in.uninstallLinks()
	in.reconcileAliases(true)
	in.removeClaudeSettings()
	in.checkRemoved()
	in.removeLeftovers()
	in.offerPackageRemoval()

	fmt.Fprint(in.stdout, "\n\033[1;32m✓ Harness AI removed.\033[0m\n\n"+uninstallGone)
	if engramKept {
		fmt.Fprint(in.stdout, "\nThe engram formula could not be removed and is still installed; the warning\nabove says so. Its configuration is gone either way.\n")
	}
	if agentBrowserKept {
		fmt.Fprint(in.stdout, "\nThe agent-browser formula could not be removed and is still installed; the\nwarning above says so. Its configuration is gone either way.\n")
	}
	fmt.Fprint(in.stdout, uninstallLeft)
	fmt.Fprintf(in.stdout, "Reinstall with: cd %s && go run ./cmd/harness bootstrap\n", in.dir.Root)
}

// removeLeftovers deletes what the harness brought onto the machine outside the managed roots: both tools' data, the backups, the status line cache and binary, and the directories only the harness filled. A directory goes only when empty, so a private entry keeps its parent.
func (in *installer) removeLeftovers() {
	for _, data := range []string{filepath.Join(in.home, ".engram"), filepath.Join(in.home, ".agent-browser")} {
		if exists(data) {
			reclaimed := size(data)
			must(os.RemoveAll(data))
			in.log("Deleted %s (%s)", data, reclaimed)
		}
	}
	in.clearBackups()
	cache := statusline.CachePath()
	removeFile(cache)
	_ = os.Remove(filepath.Dir(cache))
	empty := append(in.skillRoots(), in.commandRoots()...)
	for _, root := range in.agentRoots() {
		empty = append(empty, root[1])
	}
	empty = append(empty,
		filepath.Join(in.home, ".agents"),
		filepath.Join(in.home, ".claude", "mcp"),
		filepath.Join(in.config, "opencode", "plugins"),
		filepath.Join(in.home, ".dotfiles-backups"),
	)
	for _, dir := range empty {
		if isDir(dir) && os.Remove(dir) == nil {
			in.log("Removed empty %s", dir)
		}
	}
	// Last, since the status line setting that ran it is already gone and this may be the running binary.
	bin := filepath.Join(in.home, ".local", "bin", "harness")
	removeFile(bin)
	for _, path := range []string{filepath.Join(in.home, ".engram"), filepath.Join(in.home, ".agent-browser"), filepath.Join(in.home, ".dotfiles-backups", "harness"), bin} {
		if exists(path) || isLink(path) {
			die("Survived the uninstall: %s", path)
		}
	}
}

// offerPackageRemoval asks once before removing the shared packages still present. Anything but yes, an empty or closed stdin included, keeps them all.
func (in *installer) offerPackageRemoval() {
	var formulae []string
	if _, err := exec.LookPath("brew"); err == nil {
		installed := in.installedFormulae()
		for _, formula := range sharedPackages {
			if installed[shortName(formula)] {
				formulae = append(formulae, formula)
			}
		}
	}
	// Only Anthropic's native install, which puts the launcher in ~/.local/bin and the versions in ~/.local/share/claude; a Claude Code installed another way is not this installer's to remove.
	claudeBin, claudeData := filepath.Join(in.home, ".local", "bin", "claude"), filepath.Join(in.home, ".local", "share", "claude")
	native := exists(claudeBin) || isLink(claudeBin)
	if len(formulae) == 0 && !native {
		return
	}
	listed := make([]string, 0, len(formulae)+1)
	for _, formula := range formulae {
		listed = append(listed, shortName(formula))
	}
	if native {
		listed = append(listed, "claude (native)")
	}
	fmt.Fprintf(in.stdout, "\nThe full install also adds these when missing, and cannot tell whether it did:\n  %s\nUninstall them too? Anything else on this machine that uses them breaks. [y/N] ", strings.Join(listed, ", "))
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Fprintln(in.stdout)
	if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
		in.log("Keeping the packages")
		return
	}
	for _, formula := range formulae {
		in.uninstallFormula(shortName(formula), formula)
	}
	if native {
		removeFile(claudeBin)
		must(os.RemoveAll(claudeData))
		in.log("Removed Claude Code: %s and %s", claudeBin, claudeData)
	}
}

func removeFile(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		must(err)
	}
}

// uninstallFormula removes a formula only when Homebrew has it, and reports false only when that removal failed. A tool installed some other way stays.
func (in *installer) uninstallFormula(name, formula string) bool {
	if _, err := exec.LookPath("brew"); err != nil || exec.Command("brew", "list", "--formula", formula).Run() != nil {
		return true
	}
	remove := exec.Command("brew", "uninstall", formula)
	remove.Stdout, remove.Stderr = in.stdout, in.stderr
	if remove.Run() != nil {
		in.warn("Could not uninstall the %s formula. Remove it with: brew uninstall %s", name, formula)
		return false
	}
	return true
}

// uninstallEngram removes both halves a full install owes, registrations first because each pins the absolute path of the binary the formula removal deletes. Each client's own CLI removes its plugin and marketplace entries, best effort, since one already gone is the state being asked for. ~/.engram is the user's memory database and stays.
func (in *installer) uninstallEngram() (kept bool) {
	in.log("Removing Engram")
	if _, err := exec.LookPath("claude"); err == nil {
		_ = exec.Command("claude", "plugin", "uninstall", "engram@engram").Run()
		_ = exec.Command("claude", "plugin", "marketplace", "remove", "engram").Run()
	}
	in.removeEngramConfig()
	removeFile(filepath.Join(in.home, ".claude", "mcp", "engram.json"))
	removeFile(filepath.Join(in.config, "opencode", "plugins", "engram.ts"))
	return !in.uninstallFormula("engram", engramFormula)
}

// uninstallAgentBrowser removes both registrations before the formula, in engram's order and for its reason. A Claude Code already gone keeps its entry rather than having its config hand-edited. ~/.agent-browser holds the browser download and saved profiles and stays.
func (in *installer) uninstallAgentBrowser() (kept bool) {
	in.log("Removing agent-browser")
	if _, err := exec.LookPath("claude"); err == nil {
		_ = exec.Command("claude", "mcp", "remove", "--scope", "user", "agent-browser").Run()
	} else {
		in.warn("Claude Code is not installed, so its agent-browser entry stays.")
		in.warn("Remove it by hand from %s under mcpServers.", filepath.Join(in.home, ".claude.json"))
	}
	in.removeAgentBrowserConfig()
	return !in.uninstallFormula("agent-browser", agentBrowserFormula)
}

// removeAgentBrowserConfig is the OpenCode half, which has no CLI to delegate to. Nothing to remove writes nothing, and an emptied mcp is dropped.
func (in *installer) removeAgentBrowserConfig() {
	path := filepath.Join(in.config, "opencode", "opencode.json")
	if _, err := os.Stat(path); err != nil {
		return
	}
	data, err := os.ReadFile(path)
	var doc *object
	if err == nil {
		doc, err = parseObject(data)
	}
	if err != nil {
		die("Could not edit %s; fix it and re-run --uninstall.", path)
	}
	current, ok := doc.get("mcp")
	servers, err := parseObject(current)
	if !ok || err != nil || !servers.del("agent-browser") {
		return
	}
	if len(servers.keys) > 0 {
		doc.set("mcp", encode(servers))
	} else {
		doc.del("mcp")
	}
	must(os.WriteFile(path, render(doc), 0o644))
}

func str(raw json.RawMessage) (string, bool) {
	var s string
	return s, len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &s) == nil
}

func hasEngramPrefix(entry string) bool {
	return slices.ContainsFunc(engramToolPrefixes, func(prefix string) bool { return strings.HasPrefix(entry, prefix) })
}

func isEngramName(name string) bool {
	return name == "engram" || strings.HasPrefix(name, "engram@")
}

// dropNamed removes the matching names from the object's keys or the list's entries under key, reporting whether it removed any and whether the container is now empty. A list entry that is not a string is matched on its JSON text.
func dropNamed(doc *object, key string, matches func(string) bool) (dropped, empty bool) {
	raw, ok := doc.get(key)
	if !ok {
		return false, false
	}
	if section, err := parseObject(raw); err == nil {
		for _, name := range slices.Clone(section.keys) {
			if matches(name) {
				dropped = section.del(name)
			}
		}
		doc.set(key, encode(section))
		return dropped, len(section.keys) == 0
	}
	var list []json.RawMessage
	if raw[0] != '[' || json.Unmarshal(raw, &list) != nil {
		return false, false
	}
	kept := slices.DeleteFunc(slices.Clone(list), func(item json.RawMessage) bool {
		if name, ok := str(item); ok {
			return matches(name)
		}
		return matches(string(item))
	})
	doc.set(key, encode(kept))
	return len(kept) != len(list), len(kept) == 0
}

// pruneJSON applies prune to a client config and writes it only when prune removed something, so a file with nothing of engram's is never reformatted.
func (in *installer) pruneJSON(path string, prune func(*object) bool) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	data, err := os.ReadFile(path)
	var doc *object
	if err == nil {
		doc, err = parseObject(data)
	}
	if err != nil {
		die("%s: %v", path, err)
	}
	if !prune(doc) {
		return
	}
	must(os.WriteFile(path, render(doc), 0o644))
	fmt.Fprintf(in.stdout, "Removed the engram entries from %s\n", path)
}

// removeEngramConfig removes the entries `engram setup` writes straight into each client config, keyed on engram's names and never on position. An emptied container is dropped rather than left as {}.
func (in *installer) removeEngramConfig() {
	in.pruneJSON(in.settingsPath(), func(doc *object) bool {
		removed := false
		if raw, ok := doc.get("permissions"); ok {
			if permissions, err := parseObject(raw); err == nil {
				var allow []json.RawMessage
				if raw, ok := permissions.get("allow"); ok && raw[0] == '[' && json.Unmarshal(raw, &allow) == nil {
					kept := slices.DeleteFunc(slices.Clone(allow), func(entry json.RawMessage) bool {
						name, ok := str(entry)
						return ok && hasEngramPrefix(name)
					})
					if len(kept) != len(allow) {
						removed = true
						if len(kept) > 0 {
							permissions.set("allow", encode(kept))
						} else {
							permissions.del("allow")
						}
					}
				}
				if len(permissions.keys) == 0 {
					doc.del("permissions")
				} else {
					doc.set("permissions", encode(permissions))
				}
			}
		}
		// A plugin id is engram@engram and a marketplace is engram, so the name is matched whole rather than as a substring of somebody else's plugin.
		for _, key := range []string{"enabledPlugins", "extraKnownMarketplaces"} {
			dropped, empty := dropNamed(doc, key, isEngramName)
			removed = removed || dropped
			if empty {
				doc.del(key)
			}
		}
		return removed
	})
	in.pruneJSON(filepath.Join(in.config, "opencode", "opencode.json"), func(doc *object) bool {
		removed := false
		for _, key := range []string{"mcp", "plugin"} {
			dropped, empty := dropNamed(doc, key, func(name string) bool {
				return name == "engram" || name == "engram.ts" || strings.HasSuffix(name, "/engram.ts")
			})
			removed = removed || dropped
			if empty {
				doc.del(key)
			}
		}
		return removed
	})
	for _, name := range []string{"tui.json", "tui.jsonc"} {
		in.pruneTUI(filepath.Join(in.config, "opencode", name))
	}
}

// pruneTUI drops the subagent status line from a tui file. A file that does not parse keeps its entry with a notice: tui.jsonc may carry comments, and failing the uninstall over another tool's editor settings is worse than leaving one entry.
func (in *installer) pruneTUI(path string) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		die("%s: %v", path, err)
	}
	doc, err := parseObject(data)
	if err != nil {
		if !json.Valid(data) {
			fmt.Fprintf(in.stderr, "harness install: could not parse it, so its %s entry was left\n", subagentStatusline)
		}
		return
	}
	raw, ok := doc.get("plugin")
	var plugins []json.RawMessage
	if !ok || raw[0] != '[' || json.Unmarshal(raw, &plugins) != nil {
		return
	}
	kept := slices.DeleteFunc(slices.Clone(plugins), func(entry json.RawMessage) bool {
		name, ok := str(entry)
		return ok && name == subagentStatusline
	})
	if len(kept) == len(plugins) {
		return
	}
	if len(kept) > 0 {
		doc.set("plugin", encode(kept))
	} else {
		doc.del("plugin")
	}
	must(os.WriteFile(path, render(doc), 0o644))
	fmt.Fprintf(in.stdout, "Removed the engram entries from %s\n", path)
}

// realpath resolves a link as far as it resolves: a dangling link yields the path it names, so a dangling link into the harness still counts as the harness's.
func realpath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	target := readlink(path)
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(target)); err == nil {
		return filepath.Join(dir, filepath.Base(target))
	}
	return filepath.Clean(target)
}

// checkRemoved confirms the uninstall the way checkLinks confirms an install, so a root it silently skipped fails here rather than surfacing later as a client loading a policy from a moved checkout.
func (in *installer) checkRemoved() {
	in.checkAgentsRemoved()
	for _, root := range append(in.skillRoots(), in.commandRoots()...) {
		if !isDir(root) {
			continue
		}
		for _, entry := range entries(root) {
			if isLink(entry) && in.dir.Manages(realpath(entry)) {
				die("Managed link survived the uninstall: %s", entry)
			}
		}
	}
	for _, entry := range []string{filepath.Join(in.home, ".claude", "CLAUDE.md"), filepath.Join(in.config, "opencode", "AGENTS.md")} {
		if isLink(entry) && in.dir.Manages(readlink(entry)) {
			die("Managed link survived the uninstall: %s", entry)
		}
	}
	if rc := filepath.Join(in.home, ".zshrc"); isFile(rc) {
		text, _ := os.ReadFile(rc)
		if bytes.Contains(text, []byte(aliasMarker)) {
			die("Harness aliases survived the uninstall in %s.", rc)
		}
		if yoloAlias.Match(text) {
			die("A yolo alias survived the uninstall in %s.", rc)
		}
	}
	if isFile(in.settingsPath()) && !in.trackedSettingsHold(true) {
		die("Tracked Claude settings survived the uninstall in %s.", in.settingsPath())
	}
	in.checkEngramRemoved()
	in.checkAgentBrowserRemoved()
}

// names lists an object's keys or a list's entries as text.
func names(section any) []string {
	switch s := section.(type) {
	case map[string]any:
		return slices.Collect(maps.Keys(s))
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			out = append(out, fmt.Sprint(item))
		}
		return out
	}
	return nil
}

// checkEngramRemoved checks the containers removeEngramConfig prunes, not the whole file, so an unrelated mention of the word cannot fail the last step of a run that already deleted the formula.
func (in *installer) checkEngramRemoved() {
	for _, path := range []string{filepath.Join(in.home, ".claude", "mcp", "engram.json"), filepath.Join(in.config, "opencode", "plugins", "engram.ts")} {
		if _, err := os.Stat(path); err == nil {
			die("Engram file survived the uninstall: %s", path)
		}
	}
	survived := false
	if _, err := os.Stat(in.settingsPath()); err == nil {
		doc, ok := decodeFile(in.settingsPath())
		permissions, _ := doc["permissions"].(map[string]any)
		allow, _ := permissions["allow"].([]any)
		survived = !ok || slices.ContainsFunc(allow, func(entry any) bool {
			name, _ := entry.(string)
			return hasEngramPrefix(name)
		})
		for _, key := range []string{"enabledPlugins", "extraKnownMarketplaces"} {
			survived = survived || slices.ContainsFunc(names(doc[key]), isEngramName)
		}
	}
	opencode := filepath.Join(in.config, "opencode", "opencode.json")
	if _, err := os.Stat(opencode); err == nil {
		doc, ok := decodeFile(opencode)
		survived = survived || !ok || slices.Contains(names(doc["mcp"]), "engram") || slices.Contains(names(doc["plugin"]), "engram")
	}
	if survived {
		die("Engram entries survived the uninstall in one of the client configs.")
	}
	for _, name := range []string{"tui.json", "tui.jsonc"} {
		path := filepath.Join(in.config, "opencode", name)
		if text, err := os.ReadFile(path); isFile(path) && err == nil && bytes.Contains(text, []byte(subagentStatusline)) {
			die("The engram-enabled OpenCode statusline survived the uninstall in %s", path)
		}
	}
}

// checkAgentBrowserRemoved reads the files rather than the CLIs, so a client already gone is still checked. A survivor beside an installed Claude Code is this uninstall's failure; beside a missing one it is the documented consequence of not hand-editing another tool's config.
func (in *installer) checkAgentBrowserRemoved() {
	claudeJSON := filepath.Join(in.home, ".claude.json")
	if doc, ok := decodeFile(claudeJSON); ok {
		servers, _ := doc["mcpServers"].(map[string]any)
		if _, survived := servers["agent-browser"]; survived {
			if _, err := exec.LookPath("claude"); err == nil {
				die("The agent-browser MCP survived the uninstall in %s.", claudeJSON)
			}
			in.warn("An agent-browser entry stays in %s; its client is not installed.", claudeJSON)
		}
	}
	opencode := filepath.Join(in.config, "opencode", "opencode.json")
	if _, err := os.Stat(opencode); err == nil {
		doc, ok := decodeFile(opencode)
		servers, _ := doc["mcp"].(map[string]any)
		if _, survived := servers["agent-browser"]; !ok || survived {
			die("The agent-browser MCP survived the uninstall in %s.", opencode)
		}
	}
}
