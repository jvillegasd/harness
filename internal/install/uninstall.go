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
)

// The two MCP server id prefixes engram registers. A permission entry is engram's when it starts with one, never when it merely mentions the word.
var engramToolPrefixes = []string{"mcp__engram__", "mcp__plugin_engram_engram__"}

// subagentStatusline is the OpenCode built-in `engram setup opencode` enables, the one entry it writes that is not named after engram.
const subagentStatusline = "opencode-subagent-statusline"

var yoloAlias = regexp.MustCompile(`(?m)^[ \t]*alias (ccyolo|cxyolo|oyolo|ocyolo)=`)

const uninstallLeft = `
Not touched: the harness binary, Engram, agent-browser, and every other
package, ~/.engram, ~/.agent-browser, ~/.dotfiles-backups, and every untracked
key in the client configs.

Open a new shell to drop the aliases from the one you are in.
Reinstall with: harness install
`

// uninstallSelected removes the selected components from the selected clients: the copies no installed client still needs, then the configuration each component wrote outside them. It acts on the recorded state, so a path the harness never wrote is never touched.
func (in *installer) uninstallSelected() {
	clients, components := in.clients, in.components
	if clients == nil {
		clients = allClients
	}
	if components == nil {
		components = allComponents
	}
	if len(clients) == len(allClients) && len(components) == len(allComponents) && (len(in.loadState().Models) > 0 || len(in.loadState().Review) > 0) {
		fmt.Fprintln(in.stdout, "This also removes ~/.harness, which discards the `harness models` and `harness review level` choices.")
	}
	fmt.Fprintf(in.stdout, "Remove the harness %s from %s? [y/N] ", strings.Join(components, ", "), strings.Join(clients, ", "))
	if in.yes {
		fmt.Fprintln(in.stdout, "yes")
	} else {
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Fprintln(in.stdout)
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			in.log("Nothing removed.")
			return
		}
	}

	s := in.loadState()
	aliasesBefore := slices.ContainsFunc(allClients, func(c string) bool { return slices.Contains(s.Installed[c], "aliases") })
	for _, client := range clients {
		kept := slices.DeleteFunc(slices.Clone(s.Installed[client]), func(component string) bool {
			return slices.Contains(components, component)
		})
		if len(kept) == 0 {
			delete(s.Installed, client)
		} else {
			s.Installed[client] = kept
		}
	}
	var kept, removed []entry
	for _, e := range s.Files {
		if s.needed(e) {
			kept = append(kept, e)
			continue
		}
		in.removeManaged(e)
		removed = append(removed, e)
	}
	s.Files = kept

	selected := func(client, component string) bool {
		return slices.Contains(clients, client) && slices.Contains(components, component)
	}
	if selected("claude", "settings") {
		in.removeClaudeSettings()
	}
	if selected("codex", "settings") {
		in.reconcileCodexHooks(true)
	}
	if selected("claude", "mcp") {
		in.removeClaudeMCP()
	}
	if selected("codex", "mcp") {
		in.removeCodexMCP()
	}
	if selected("opencode", "mcp") {
		in.removeOpenCodeMCP()
	}
	aliasesGone := !slices.ContainsFunc(allClients, func(c string) bool { return slices.Contains(s.Installed[c], "aliases") })
	// The block goes once no client keeps aliases; an uninstall of every client's aliases removes it even without a recorded install.
	aliasesRemoved := aliasesGone && slices.Contains(components, "aliases") && (aliasesBefore || len(clients) == len(allClients))
	if aliasesRemoved {
		in.reconcileAliases(true)
	}
	if len(s.Installed) == 0 {
		must(os.RemoveAll(in.stateDir()))
	} else {
		in.saveState(in.keepChoices(s))
	}
	in.checkRemoved(removed, selected, aliasesRemoved)

	fmt.Fprint(in.stdout, "\n\033[1;32m✓ Harness AI removed.\033[0m\n"+uninstallLeft)
}

func removeFile(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		must(err)
	}
}

// removeClaudeMCP removes what Engram's claude-code setup and the agent-browser registration wrote into Claude Code. Each CLI removes its own entries, best effort, since one already gone is the state being asked for; a Claude Code already gone keeps its agent-browser entry rather than having its config hand-edited.
func (in *installer) removeClaudeMCP() {
	in.log("Removing Engram and agent-browser from Claude Code")
	if _, err := exec.LookPath("claude"); err == nil {
		_ = exec.Command("claude", "plugin", "uninstall", "engram@engram").Run()
		_ = exec.Command("claude", "plugin", "marketplace", "remove", "engram").Run()
		_ = exec.Command("claude", "mcp", "remove", "--scope", "user", "agent-browser").Run()
		_ = exec.Command("claude", "mcp", "remove", "--scope", "user", "engram").Run()
	} else {
		in.warn("Claude Code is not installed, so its engram and agent-browser entries stay.")
		in.warn("Remove them by hand from %s under mcpServers.", filepath.Join(in.home, ".claude.json"))
	}
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
	removeFile(filepath.Join(in.home, ".claude", "mcp", "engram.json"))
}

// removeCodexMCP removes what Engram's codex setup and the agent-browser registration wrote into ~/.codex, through Codex's own CLI and best effort, since one already gone is the state being asked for. Engram's files go whether or not Codex is installed; a Codex already gone keeps its config entries rather than having its config hand-edited.
func (in *installer) removeCodexMCP() {
	in.log("Removing Engram and agent-browser from Codex")
	for _, name := range codexEngramFiles {
		removeFile(filepath.Join(in.codexHome(), name))
	}
	if _, err := exec.LookPath("codex"); err != nil {
		in.warn("Codex is not installed, so its engram and agent-browser entries stay in %s.", filepath.Join(in.codexHome(), "config.toml"))
		return
	}
	_ = in.codex("mcp", "remove", "agent-browser").Run()
	_ = in.codex("mcp", "remove", "engram").Run()
	_ = in.codex("plugin", "remove", "engram@engram").Run()
	_ = in.codex("plugin", "marketplace", "remove", "engram").Run()
}

// codexEngramFiles are what `engram setup codex` writes beside config.toml.
var codexEngramFiles = []string{"engram-instructions.md", "engram-compact-prompt.md"}

// checkCodexMCPRemoved asks Codex, which reads its own config, rather than parsing the TOML; with Codex gone there is nothing to ask and the warning above stands.
func (in *installer) checkCodexMCPRemoved() {
	for _, name := range codexEngramFiles {
		if path := filepath.Join(in.codexHome(), name); exists(path) {
			die("Engram file survived the uninstall: %s", path)
		}
	}
	if _, err := exec.LookPath("codex"); err != nil {
		return
	}
	for _, server := range []string{"agent-browser", "engram"} {
		get := in.codex("mcp", "get", server)
		get.Stderr = nil
		if get.Run() == nil {
			die("The %s MCP survived the uninstall in %s.", server, filepath.Join(in.codexHome(), "config.toml"))
		}
	}
}

// removeOpenCodeMCP removes what Engram's opencode setup and the agent-browser registration wrote into OpenCode, which has no CLI to delegate to. An emptied container is dropped rather than left as {}.
func (in *installer) removeOpenCodeMCP() {
	in.log("Removing Engram and agent-browser from OpenCode")
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
	removeFile(filepath.Join(in.config, "opencode", "plugins", "engram.ts"))
	in.removeAgentBrowserConfig()
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

// checkRemoved confirms the uninstall the way checkInstall confirms an install: each removed copy is gone unless the user had edited it, and each selected configuration is gone.
func (in *installer) checkRemoved(removed []entry, selected func(client, component string) bool, aliasesRemoved bool) {
	for _, e := range removed {
		if current := digest(e.Path); current != "" && current == e.Digest {
			die("Managed copy survived the uninstall: %s", e.Path)
		}
	}
	if aliasesRemoved {
		if rc := filepath.Join(in.home, ".zshrc"); isFile(rc) {
			text, _ := os.ReadFile(rc)
			if bytes.Contains(text, []byte(aliasMarker)) {
				die("Harness aliases survived the uninstall in %s.", rc)
			}
			if yoloAlias.Match(text) {
				die("A yolo alias survived the uninstall in %s.", rc)
			}
		}
	}
	if selected("claude", "settings") && isFile(in.settingsPath()) && !in.trackedSettingsHold(true) {
		die("Tracked Claude settings survived the uninstall in %s.", in.settingsPath())
	}
	if selected("codex", "settings") && !in.codexHooksHold(true) {
		die("The harness compaction hook survived the uninstall in %s.", in.codexHooksPath())
	}
	if selected("claude", "mcp") {
		in.checkClaudeMCPRemoved()
	}
	if selected("codex", "mcp") {
		in.checkCodexMCPRemoved()
	}
	if selected("opencode", "mcp") {
		in.checkOpenCodeMCPRemoved()
	}
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

// checkClaudeMCPRemoved checks the containers removeClaudeMCP prunes, not the whole file, so an unrelated mention of the word cannot fail the run. The agent-browser and engram entries are read from the file rather than the CLI; a survivor beside a missing Claude Code is the documented consequence of not hand-editing another tool's config.
func (in *installer) checkClaudeMCPRemoved() {
	if path := filepath.Join(in.home, ".claude", "mcp", "engram.json"); exists(path) {
		die("Engram file survived the uninstall: %s", path)
	}
	if _, err := os.Stat(in.settingsPath()); err == nil {
		doc, ok := decodeFile(in.settingsPath())
		permissions, _ := doc["permissions"].(map[string]any)
		allow, _ := permissions["allow"].([]any)
		survived := !ok || slices.ContainsFunc(allow, func(entry any) bool {
			name, _ := entry.(string)
			return hasEngramPrefix(name)
		})
		for _, key := range []string{"enabledPlugins", "extraKnownMarketplaces"} {
			survived = survived || slices.ContainsFunc(names(doc[key]), isEngramName)
		}
		if survived {
			die("Engram entries survived the uninstall in %s.", in.settingsPath())
		}
	}
	claudeJSON := filepath.Join(in.home, ".claude.json")
	if doc, ok := decodeFile(claudeJSON); ok {
		servers, _ := doc["mcpServers"].(map[string]any)
		for _, server := range []string{"agent-browser", "engram"} {
			if _, survived := servers[server]; !survived {
				continue
			}
			if _, err := exec.LookPath("claude"); err == nil {
				die("The %s MCP survived the uninstall in %s.", server, claudeJSON)
			}
			in.warn("An %s entry stays in %s; its client is not installed.", server, claudeJSON)
		}
	}
}

func (in *installer) checkOpenCodeMCPRemoved() {
	if path := filepath.Join(in.config, "opencode", "plugins", "engram.ts"); exists(path) {
		die("Engram file survived the uninstall: %s", path)
	}
	opencode := filepath.Join(in.config, "opencode", "opencode.json")
	if _, err := os.Stat(opencode); err == nil {
		doc, ok := decodeFile(opencode)
		servers, _ := doc["mcp"].(map[string]any)
		_, browser := servers["agent-browser"]
		if !ok || browser || slices.Contains(names(doc["mcp"]), "engram") || slices.Contains(names(doc["plugin"]), "engram") {
			die("Engram or agent-browser entries survived the uninstall in %s.", opencode)
		}
	}
	for _, name := range []string{"tui.json", "tui.jsonc"} {
		path := filepath.Join(in.config, "opencode", name)
		if text, err := os.ReadFile(path); isFile(path) && err == nil && bytes.Contains(text, []byte(subagentStatusline)) {
			die("The engram-enabled OpenCode statusline survived the uninstall in %s", path)
		}
	}
}
