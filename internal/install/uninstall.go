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

	"github.com/jvillegasd/harness/internal/ui"
)

var engramToolPrefixes = []string{"mcp__engram__", "mcp__plugin_engram_engram__"}

const subagentStatusline = "opencode-subagent-statusline"

var yoloAlias = regexp.MustCompile(`(?m)^[ \t]*alias (ccyolo|cxyolo|oyolo|ocyolo)=`)

const uninstallLeft = "Not touched: every tool's data (~/.engram, ~/.agent-browser, ~/.claude, ~/.codex), ~/.dotfiles-backups, and every untracked key in the client configs. Reinstall with: harness install"

func (in *installer) uninstallSelected() {
	defer in.flushOnPanic()

	in.usePATH()
	clients := in.clients
	if clients == nil {
		clients = allClients
	}
	clients = inOrder(clients, allClients)
	components := in.components
	switch {
	case in.all:
		components = allComponents
	case components == nil:
		components = configComponents
	}
	components = inOrder(components, allComponents)
	configs := slices.DeleteFunc(slices.Clone(components), func(c string) bool { return !slices.Contains(configComponents, c) })

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

	var removals []tool
	var notes []string
	for i := len(tools) - 1; i >= 0; i-- {
		t := tools[i]
		if !in.all && !slices.Contains(in.components, t.component) || t.client != "" && !slices.Contains(clients, t.client) {
			continue
		}
		_, recorded := s.Tools[t.name]
		dependents := installedThrough(t.name, s.Tools, removals)
		switch {
		case !recorded:
			if present(t.name) {
				notes = append(notes, t.name+": present before the harness, left installed")
			}
		case t.client == "" && len(s.Installed) > 0:
			notes = append(notes, fmt.Sprintf("%s: still used by %s, left installed", t.name, strings.Join(inOrder(slices.Collect(maps.Keys(s.Installed)), allClients), ", ")))
		case len(dependents) > 0:
			notes = append(notes, fmt.Sprintf("%s: %s was installed through it, left installed", t.name, strings.Join(dependents, ", ")))
		default:
			removals = append(removals, t)
		}
	}
	var names []string
	for _, t := range removals {
		names = append(names, t.name)
	}
	if len(configs) == 0 && len(removals) == 0 {
		in.drawList("Left in place", notes)
		fmt.Fprintln(in.stdout)
		in.log("Nothing to remove.")
		return
	}
	emptied := len(s.Installed) == 0 && len(s.Tools) == len(removals)
	if emptied && (len(s.Models) > 0 || len(s.Review) > 0) {
		fmt.Fprintln(in.stdout, in.style().Warn("This also removes ~/.harness, which discards the `harness models` and `harness review level` choices."))
	}
	var parts []string
	if len(configs) > 0 {
		parts = append(parts, fmt.Sprintf("Remove the harness %s from %s", strings.Join(configs, ", "), strings.Join(clients, ", ")))
	}
	if len(removals) > 0 {
		parts = append(parts, "uninstall "+strings.Join(names, ", "))
	}
	if in.all && emptied {
		parts = append(parts, "delete "+in.binaryPath())
	}
	question := strings.Join(parts, ", and ")
	fmt.Fprintf(in.stdout, "%s%s? [y/N] ", strings.ToUpper(question[:1]), question[1:])
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

	var kept, removed []entry
	in.book.deferring = true
	for _, e := range s.Files {
		in.book.component = e.Component
		if s.needed(e) {
			kept = append(kept, e)
			continue
		}
		in.removeManaged(e)
		removed = append(removed, e)
	}
	in.book.deferring = false
	in.book.component = ""
	s.Files = kept
	if len(configs) > 0 || len(in.book.deferred) > 0 {
		in.begin("files", strings.Join(configs, ", ")+" from "+strings.Join(clients, ", "))
		for _, c := range configs {
			if in.book.files[c] != nil {
				in.log("%s  %s", ui.Pad(c, 8), in.style().Dim(in.counted(c)))
			}
		}
		in.flush(append(configComponents, "")...)
		in.end(in.counted(configs...))
		if slices.ContainsFunc(configs, func(c string) bool { return c == "settings" || c == "mcps" || c == "aliases" }) {
			in.begin("config", "client settings, MCP registrations, and aliases")
		}
	}

	selected := func(client, component string) bool {
		return slices.Contains(clients, client) && slices.Contains(configs, component)
	}
	if selected("claude", "settings") {
		in.removeClaudeSettings()
	}
	if selected("codex", "settings") {
		in.reconcileCodexHooks(true)
	}
	if selected("opencode", "settings") {
		in.reconcileOpenCodePermission(true)
	}
	if selected("claude", "mcps") {
		in.removeClaudeMCP()
	}
	if selected("codex", "mcps") {
		in.removeCodexMCP()
	}
	if selected("opencode", "mcps") {
		in.removeOpenCodeMCP()
	}
	aliasesGone := !slices.ContainsFunc(allClients, func(c string) bool { return slices.Contains(s.Installed[c], "aliases") })

	aliasesRemoved := aliasesGone && slices.Contains(configs, "aliases") && (aliasesBefore || len(clients) == len(allClients))
	if aliasesRemoved {
		in.reconcileAliases(true)
	}
	in.updateState(func(now *state) { now.Installed, now.Files = s.Installed, s.Files })
	in.end("reconciled")
	in.checkRemoved(removed, selected, aliasesRemoved)

	if len(removals) > 0 {
		in.begin("tools", strings.Join(names, ", "))
		for _, t := range removals {
			in.removeTool(t, s.Tools[t.name])
		}
		in.end("removed")
	}
	if in.loadState().empty() {
		must(os.RemoveAll(in.stateDir()))
		if in.all {
			removeFile(in.binaryPath())
			in.log("Removed %s", in.binaryPath())
		}
	}

	in.summarize("Harness AI removed.")
	for _, t := range removals {
		if path := filepath.Join(in.home, t.data); t.data != "" && exists(path) {
			notes = append(notes, fmt.Sprintf("%s: its data stays in %s", t.name, path))
		}
	}
	in.drawList("Left in place", notes)
	if aliasesRemoved {
		fmt.Fprintln(in.stdout, "\n"+in.style().Dim("Open a new shell to drop the aliases from the one you are in."))
	}
	fmt.Fprintln(in.stdout, "\n"+in.style().Dim(uninstallLeft))
}

func removeFile(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		must(err)
	}
}

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

var codexEngramFiles = []string{"engram-instructions.md", "engram-compact-prompt.md"}

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
		die("Could not edit %s; fix it and re-run harness uninstall.", path)
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
			in.warn("%s: could not parse it, so its %s entry was left", path, subagentStatusline)
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

func (in *installer) checkRemoved(removed []entry, selected func(client, component string) bool, aliasesRemoved bool) {
	for _, e := range removed {
		if current := digest(e.Path); current != "" && current == e.Digest {
			die("Managed copy survived the uninstall: %s", e.Path)
		}
	}
	if aliasesRemoved {
		for _, rc := range in.aliasesStripped {
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
	if selected("opencode", "settings") && !in.openCodePermissionHold(true) {
		die("The harness permission rules survived the uninstall in %s.", in.openCodeConfigPath())
	}
	if selected("claude", "mcps") {
		in.checkClaudeMCPRemoved()
	}
	if selected("codex", "mcps") {
		in.checkCodexMCPRemoved()
	}
	if selected("opencode", "mcps") {
		in.checkOpenCodeMCPRemoved()
	}
}

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
