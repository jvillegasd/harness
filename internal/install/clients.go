package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/jvillegasd/harness/internal/models"
)

// object is a JSON object that keeps its key order, so rewriting a client config moves only what changed.
type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func newObject() *object {
	return &object{vals: map[string]json.RawMessage{}}
}

// parseObject reads a JSON object in key order. A repeated key keeps its first position and its last value.
func parseObject(data []byte) (*object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if tok != json.Delim('{') {
		return nil, errors.New("expected a JSON object")
	}
	o := newObject()
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		key, _ := tok.(string)
		o.set(key, value)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after the JSON object")
	}
	return o, nil
}

func (o *object) get(key string) (json.RawMessage, bool) {
	value, ok := o.vals[key]
	return value, ok
}

func (o *object) set(key string, value json.RawMessage) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = value
}

func (o *object) del(key string) bool {
	if _, ok := o.vals[key]; !ok {
		return false
	}
	delete(o.vals, key)
	o.keys = slices.DeleteFunc(o.keys, func(k string) bool { return k == key })
	return true
}

func (o *object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(encode(key))
		b.WriteByte(':')
		b.Write(o.vals[key])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// encode marshals without HTML escaping, so <, >, and & stay as the user wrote them.
func encode(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	must(enc.Encode(v))
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// render formats a document with two-space indents and non-ASCII escaped as \uXXXX, matching the configs earlier installs wrote, with one trailing newline.
func render(v any) []byte {
	var b bytes.Buffer
	must(json.Indent(&b, encode(v), "", "  "))
	var out strings.Builder
	for _, r := range b.String() {
		if r < 0x80 {
			out.WriteRune(r)
			continue
		}
		for _, unit := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&out, `\u%04x`, unit)
		}
	}
	out.WriteByte('\n')
	return []byte(out.String())
}

// same reports whether two JSON texts hold the same document, so a file that only differs in formatting is never rewritten.
func same(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func decodeFile(path string) (map[string]any, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var doc map[string]any
	return doc, json.Unmarshal(data, &doc) == nil
}

func (in *installer) settingsPath() string {
	return filepath.Join(in.home, ".claude", "settings.json")
}

// readSettings parses the live settings and then clients/claude.json, naming whichever file is broken: the user's own, or the repository's.
func (in *installer) readSettings(path string) (live *object, before []byte, tracked *object) {
	before, err := os.ReadFile(path)
	if err == nil {
		live, err = parseObject(before)
	}
	if err != nil {
		die("%s is not a JSON object; fix it and re-run.", path)
	}
	trackedPath := filepath.Join(in.dir.Clients(), "claude.json")
	data, err := os.ReadFile(trackedPath)
	if err == nil {
		tracked, err = parseObject(data)
	}
	if err != nil {
		die("%s is unreadable or not a JSON object.", trackedPath)
	}
	return live, before, tracked
}

// writeIfChanged writes a reconciled document, after a backup, only when it differs from what the file held.
func (in *installer) writeIfChanged(path string, before, after []byte, message string) {
	if same(before, after) {
		return
	}
	in.backupCopy(path)
	must(os.WriteFile(path, after, 0o644))
	in.log(message, path)
}

// harnessCommand marks a hook the harness owns: one that runs the installed harness binary.
const harnessCommand = `.local/bin/harness"`

// harnessGroup reports whether a hook matcher group runs the installed harness, which is how a reconcile tells the harness's group from one the user or another tool wrote under the same event.
func harnessGroup(raw json.RawMessage) bool {
	var group struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(raw, &group) != nil {
		return false
	}
	return slices.ContainsFunc(group.Hooks, func(h struct {
		Command string `json:"command"`
	}) bool {
		return strings.Contains(h.Command, harnessCommand)
	})
}

// mergeHooks returns the live hooks with every harness group under the tracked events replaced by the tracked groups, or with add false, only removed. Other events and other groups under the same event stay, in order. An event or a hooks object left empty by a removal goes.
func mergeHooks(live, tracked json.RawMessage, add bool) json.RawMessage {
	merged, err := parseObject(live)
	if err != nil {
		merged = newObject()
	}
	events, err := parseObject(tracked)
	must(err)
	for _, event := range events.keys {
		var kept []json.RawMessage
		if current, ok := merged.get(event); ok {
			var groups []json.RawMessage
			if json.Unmarshal(current, &groups) != nil {
				continue
			}
			kept = slices.DeleteFunc(groups, harnessGroup)
		}
		if add {
			var ours []json.RawMessage
			must(json.Unmarshal(events.vals[event], &ours))
			kept = append(kept, ours...)
		}
		if len(kept) == 0 {
			merged.del(event)
		} else {
			merged.set(event, encode(kept))
		}
	}
	return encode(merged)
}

// reconcileClaudeSettings merges clients/claude.json into the live settings and leaves every other key alone, because other tools, engram setup, and the client all write machine state there. A tracked key replaces the live value, so deleting one from the tracked file deletes it from the machine. permissions merges one level down: deny and ask stay owned, and allow gains the tracked rules beside the ones engram setup and the user write. hooks keeps every hook the harness does not run, so the user's and other tools' hooks under the same event survive.
func (in *installer) reconcileClaudeSettings() {
	path := in.settingsPath()
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		must(os.WriteFile(path, []byte("{}\n"), 0o644))
	}
	live, before, tracked := in.readSettings(path)
	for _, key := range tracked.keys {
		value := tracked.vals[key]
		if current, ok := live.get(key); ok && key == "permissions" {
			if merged, err := parseObject(current); err == nil {
				add, err := parseObject(value)
				must(err)
				for _, name := range add.keys {
					entries := add.vals[name]
					if name == "allow" {
						entries = mergeAllow(merged.vals[name], entries, true)
					}
					merged.set(name, entries)
				}
				value = encode(merged)
			}
		}
		if key == "hooks" {
			current, _ := live.get(key)
			value = mergeHooks(current, value, true)
		}
		live.set(key, value)
	}
	in.writeIfChanged(path, before, render(live), "Reconciled the tracked Claude settings in %s")
}

// mergeAllow adds the tracked allow rules to the live ones, keeping the live order and every rule engram setup or the user wrote, or with add false, removes only the tracked rules. A removal that leaves nothing returns nil, so the class goes. Live entries that are not a list are replaced on add and dropped on removal.
func mergeAllow(live, tracked json.RawMessage, add bool) json.RawMessage {
	var have, want []json.RawMessage
	_ = json.Unmarshal(live, &have)
	_ = json.Unmarshal(tracked, &want)
	tracks := func(entry json.RawMessage) bool {
		return slices.ContainsFunc(want, func(w json.RawMessage) bool { return string(w) == string(entry) })
	}
	kept := slices.DeleteFunc(slices.Clone(have), tracks)
	if add {
		kept = append(kept, want...)
	}
	if len(kept) == 0 {
		return nil
	}
	return encode(kept)
}

// removeClaudeSettings deletes exactly what reconcileClaudeSettings writes: the tracked keys, and within permissions only the tracked classes. A permissions the harness emptied goes too.
func (in *installer) removeClaudeSettings() {
	path := in.settingsPath()
	if !isFile(path) {
		return
	}
	live, before, tracked := in.readSettings(path)
	for _, key := range tracked.keys {
		current, ok := live.get(key)
		if ok && key == "hooks" {
			if rest := mergeHooks(current, tracked.vals[key], false); string(rest) != "{}" {
				live.set(key, rest)
				continue
			}
		}
		owned, err := parseObject(current)
		if !ok || key != "permissions" || err != nil {
			live.del(key)
			continue
		}
		if classes, err := parseObject(tracked.vals[key]); err == nil {
			for _, name := range classes.keys {
				if name == "allow" {
					if rest := mergeAllow(owned.vals[name], classes.vals[name], false); rest != nil {
						owned.set(name, rest)
						continue
					}
				}
				owned.del(name)
			}
		}
		if len(owned.keys) == 0 {
			live.del(key)
		} else {
			live.set(key, encode(owned))
		}
	}
	in.writeIfChanged(path, before, render(live), "Removed the tracked Claude settings from %s")
}

// trackedSettingsHold reports whether every tracked key reached the live settings, or with removed, whether every one is gone while the untracked keys beside them stay.
func (in *installer) trackedSettingsHold(removed bool) bool {
	live, liveOK := decodeFile(in.settingsPath())
	tracked, trackedOK := decodeFile(filepath.Join(in.dir.Clients(), "claude.json"))
	if !liveOK || !trackedOK {
		return false
	}
	holds := func(have map[string]any, key string, want any) bool {
		value, present := have[key]
		if removed {
			return !present
		}
		return reflect.DeepEqual(value, want)
	}
	for key, want := range tracked {
		if key == "hooks" {
			if !hooksHold(live[key], want, removed) {
				return false
			}
			continue
		}
		if key != "permissions" {
			if !holds(live, key, want) {
				return false
			}
			continue
		}
		owned, _ := live[key].(map[string]any)
		classes, _ := want.(map[string]any)
		for name, entry := range classes {
			if name == "allow" {
				have, _ := owned[name].([]any)
				for _, rule := range entry.([]any) {
					if slices.Contains(have, rule) == removed {
						return false
					}
				}
				continue
			}
			if !holds(owned, name, entry) {
				return false
			}
		}
	}
	return true
}

// hooksHold reports whether every tracked hook group sits under its event in the live hooks, or with removed, whether no harness group is left under a tracked event.
func hooksHold(live, tracked any, removed bool) bool {
	have, _ := live.(map[string]any)
	events, _ := tracked.(map[string]any)
	for event, want := range events {
		groups, _ := have[event].([]any)
		for _, group := range want.([]any) {
			present := slices.ContainsFunc(groups, func(g any) bool { return reflect.DeepEqual(g, group) })
			if present == removed {
				return false
			}
		}
		if removed {
			for _, g := range groups {
				if raw, err := json.Marshal(g); err == nil && harnessGroup(raw) {
					return false
				}
			}
		}
	}
	return true
}

func (in *installer) codexHooksPath() string {
	return filepath.Join(in.codexHome(), "hooks.json")
}

// reconcileCodexHooks merges the hooks in clients/codex/hooks.json into ~/.codex/hooks.json, which the user and other tools may also write, the way reconcileClaudeSettings merges Claude Code's: only the harness's own groups are replaced, and every other key and hook stays. With remove it takes the harness's groups out, and deletes a file left with nothing in it.
func (in *installer) reconcileCodexHooks(remove bool) {
	path := in.codexHooksPath()
	if remove && !isFile(path) {
		return
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		data = []byte("{}\n")
	} else if err != nil {
		die("Reading %s failed: %v", path, err)
	}
	live, err := parseObject(data)
	if err != nil {
		die("%s is not a JSON object, so the harness compaction hook cannot be merged into it: %v", path, err)
	}
	tracked, err := os.ReadFile(filepath.Join(in.dir.Clients(), "codex", "hooks.json"))
	must(err)
	want, err := parseObject(tracked)
	must(err)
	current, _ := live.get("hooks")
	if merged := mergeHooks(current, want.vals["hooks"], !remove); string(merged) == "{}" {
		live.del("hooks")
	} else {
		live.set("hooks", merged)
	}
	if remove && len(live.keys) == 0 {
		in.backupCopy(path)
		must(os.Remove(path))
		in.log("Removed the harness compaction hook and the now empty %s", path)
		return
	}
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	in.writeIfChanged(path, data, render(live), "Reconciled the harness compaction hook in %s")
}

// codexHooksHold reports whether the tracked hook groups are in ~/.codex/hooks.json, or with removed, whether no harness group is left there.
func (in *installer) codexHooksHold(removed bool) bool {
	tracked, ok := decodeFile(filepath.Join(in.dir.Clients(), "codex", "hooks.json"))
	if !ok {
		return false
	}
	live, ok := decodeFile(in.codexHooksPath())
	if !ok {
		return removed && !isFile(in.codexHooksPath())
	}
	return hooksHold(live["hooks"], tracked["hooks"], removed)
}

func (in *installer) openCodeConfigPath() string {
	return filepath.Join(in.config, "opencode", "opencode.json")
}

// trackedOpenCodePermission reads clients/opencode/permission.json: one object per permission class (read, edit, bash), each mapping a pattern to its action.
func (in *installer) trackedOpenCodePermission() *object {
	path := filepath.Join(in.dir.Clients(), "opencode", "permission.json")
	data, err := os.ReadFile(path)
	if err != nil {
		die("Reading %s failed: %v", path, err)
	}
	tracked, err := parseObject(data)
	if err != nil {
		die("%s is not a JSON object: %v", path, err)
	}
	return tracked
}

// reconcileOpenCodePermission merges the tracked permission rules into opencode.json, which Engram, the agent-browser registration, and the user also write, so every other key and rule stays. OpenCode applies the last rule that matches, so within each class the harness's patterns are moved to the end, after the user's, keeping the user's action where it is stricter; a class or a whole permission the user set to one action becomes {"*": action} first, which OpenCode reads the same way. With remove it takes out only the tracked patterns that still hold their tracked action, and drops a class or permission that leaves empty.
func (in *installer) reconcileOpenCodePermission(remove bool) {
	path := in.openCodeConfigPath()
	if remove && !isFile(path) {
		return
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		data = []byte("{}\n")
	} else if err != nil {
		die("Reading %s failed: %v", path, err)
	}
	live, err := parseObject(data)
	if err != nil {
		die("%s is not a JSON object, so the harness permission rules cannot be merged into it: %v", path, err)
	}
	current, _ := live.get("permission")
	merged, err := mergePermission(current, in.trackedOpenCodePermission(), !remove)
	if err != nil {
		die("%s: %v; fix it and re-run.", path, err)
	}
	if merged == nil {
		live.del("permission")
	} else {
		live.set("permission", merged)
	}
	// Compared in key order, not with writeIfChanged's same: moving the harness's rules behind a rule the user added later changes only the order, and in OpenCode the order decides which rule applies.
	before, _ := parseObject(data)
	after := render(live)
	if before != nil && bytes.Equal(render(before), after) {
		return
	}
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	in.backupCopy(path)
	must(os.WriteFile(path, after, 0o644))
	in.log("Reconciled the harness permission rules in %s", path)
}

// actionRules reads a permission value as pattern rules: an object as it is, a single action string as {"*": action}, and an absent or null value as no rules.
func actionRules(raw json.RawMessage) (*object, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return newObject(), nil
	}
	var action string
	if json.Unmarshal(raw, &action) == nil {
		rules := newObject()
		rules.set("*", encode(action))
		return rules, nil
	}
	return parseObject(raw)
}

// strictness ranks an OpenCode action, so a rule the user made stricter than the harness's is never weakened: allow, then ask, then deny. Anything else ranks below allow.
func strictness(raw json.RawMessage) int {
	var action string
	_ = json.Unmarshal(raw, &action)
	return slices.Index([]string{"allow", "ask", "deny"}, action)
}

// mergePermission returns the live permission with every tracked pattern moved to the end of its class in the tracked order, keeping the user's action where it is stricter than the tracked one; or with add false, with only the tracked patterns that still hold their tracked action removed, so a rule the user set differently stays. It returns nil when nothing is left.
func mergePermission(live json.RawMessage, tracked *object, add bool) (json.RawMessage, error) {
	permission, err := actionRules(live)
	if err != nil {
		return nil, errors.New("permission is neither an action nor a JSON object")
	}
	for _, class := range tracked.keys {
		ours, err := parseObject(tracked.vals[class])
		must(err)
		rules, err := actionRules(permission.vals[class])
		if err != nil {
			return nil, fmt.Errorf("permission.%s is neither an action nor a JSON object", class)
		}
		stricter := map[string]json.RawMessage{}
		for _, pattern := range ours.keys {
			have, had := rules.get(pattern)
			if !had {
				continue
			}
			if add && strictness(have) > strictness(ours.vals[pattern]) {
				stricter[pattern] = have
			}
			if add || same(have, ours.vals[pattern]) {
				rules.del(pattern)
			}
		}
		if add {
			for _, pattern := range ours.keys {
				value := ours.vals[pattern]
				if have, ok := stricter[pattern]; ok {
					value = have
				}
				rules.set(pattern, value)
			}
		}
		if len(rules.keys) == 0 {
			permission.del(class)
		} else {
			permission.set(class, encode(rules))
		}
	}
	if len(permission.keys) == 0 {
		return nil, nil
	}
	return encode(permission), nil
}

// openCodePermissionHold reports whether every tracked pattern ends its class in opencode.json, in order, with its tracked action or a stricter one, so no rule of the user's comes after it; or with removed, whether no tracked pattern still holds its tracked action there.
func (in *installer) openCodePermissionHold(removed bool) bool {
	path := in.openCodeConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return removed && errors.Is(err, fs.ErrNotExist)
	}
	live, err := parseObject(data)
	if err != nil {
		return false
	}
	current, _ := live.get("permission")
	permission, err := actionRules(current)
	if err != nil {
		return false
	}
	tracked := in.trackedOpenCodePermission()
	for _, class := range tracked.keys {
		ours, err := parseObject(tracked.vals[class])
		must(err)
		rules, err := actionRules(permission.vals[class])
		if err != nil {
			return false
		}
		if removed {
			if slices.ContainsFunc(ours.keys, func(p string) bool { have, ok := rules.get(p); return ok && same(have, ours.vals[p]) }) {
				return false
			}
			continue
		}
		if len(rules.keys) < len(ours.keys) {
			return false
		}
		tail := rules.keys[len(rules.keys)-len(ours.keys):]
		for i, pattern := range ours.keys {
			if tail[i] != pattern || strictness(rules.vals[pattern]) < strictness(ours.vals[pattern]) {
				return false
			}
		}
	}
	return true
}

const aliasMarker = "# portable-harness aliases"

var (
	aliasLine = regexp.MustCompile(`^\s*alias\s+([A-Za-z0-9_-]+)=`)
	lineBreak = regexp.MustCompile(`\r\n|\r|\n`)
)

// aliasRCs are every startup file an install may have written the managed line into, which an uninstall clears.
func (in *installer) aliasRCs() []string {
	var out []string
	for _, name := range []string{".zshrc", ".bashrc", ".bash_profile", ".bash_login", ".profile"} {
		out = append(out, filepath.Join(in.home, name))
	}
	return out
}

// aliasRC is the startup file the login shell reads, chosen by $SHELL or, when it is unset, by the OS default shell: zsh on macOS, bash on Linux. It is empty for a shell that cannot source aliases.sh, such as fish.
func (in *installer) aliasRC() string {
	shell := filepath.Base(os.Getenv("SHELL"))
	if os.Getenv("SHELL") == "" {
		shell = map[bool]string{true: "zsh", false: "bash"}[runtime.GOOS == "darwin"]
	}
	switch shell {
	case "zsh":
		return filepath.Join(in.home, ".zshrc")
	case "bash":
		if runtime.GOOS != "darwin" {
			return filepath.Join(in.home, ".bashrc")
		}
		// macOS terminals open login shells, which read only the first of these that exists, so a new ~/.bash_profile would hide an existing ~/.profile.
		for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
			if path := filepath.Join(in.home, name); isFile(path) {
				return path
			}
		}
		return filepath.Join(in.home, ".bash_profile")
	}
	return ""
}

// reconcileAliases writes the one managed line that sources the installed aliases.sh into the login shell's startup file, or with remove strips it from every startup file that carries it, so the committed file stays the only definition.
func (in *installer) reconcileAliases(remove bool) {
	if remove {
		// Only a file carrying the marker is the harness's to edit; a hand-written alias elsewhere is the user's.
		for _, rc := range in.aliasRCs() {
			if text, err := os.ReadFile(rc); err == nil && strings.Contains(string(text), aliasMarker) {
				in.reconcileAliasesIn(rc, true)
				in.aliasesStripped = append(in.aliasesStripped, rc)
			}
		}
		return
	}
	rc := in.aliasRC()
	if rc == "" {
		in.warn("Your login shell is %s, which has no startup file the harness writes. The yolo aliases are in %s, written for bash and zsh.", os.Getenv("SHELL"), in.aliasesPath())
		return
	}
	in.reconcileAliasesIn(rc, false)
}

// reconcileAliasesIn writes or strips the managed line in one startup file. Hand-written copies of the managed aliases go either way, the retired oyolo included. A removal never creates a missing file, and one that finds nothing writes nothing.
func (in *installer) reconcileAliasesIn(rc string, remove bool) {
	if !isFile(rc) {
		if remove {
			return
		}
		f, err := os.OpenFile(rc, os.O_CREATE|os.O_WRONLY, 0o644)
		must(err)
		must(f.Close())
	}
	data, err := os.ReadFile(rc)
	if err != nil {
		die("Could not reconcile aliases in %s.", rc)
	}
	current := string(data)
	source := in.aliasesPath()
	// No empty entry after a final line break.
	lines := lineBreak.Split(current, -1)
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var kept []string
	dropped := false
	for i, line := range lines {
		alias := aliasLine.FindStringSubmatch(line)
		managed := alias != nil && (alias[1] == "ccyolo" || alias[1] == "cxyolo" || alias[1] == "oyolo" || alias[1] == "ocyolo")
		// The line after the marker is the managed source line whatever path an earlier install wrote into it.
		afterMarker := i > 0 && strings.Contains(lines[i-1], aliasMarker)
		if managed || afterMarker || strings.Contains(line, aliasMarker) || strings.Contains(line, source) {
			dropped = true
			continue
		}
		kept = append(kept, line)
	}
	if remove && !dropped {
		return
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if !remove {
		// The POSIX dot, because ~/.profile is also read by sh, which has no source.
		kept = append(kept, aliasMarker, fmt.Sprintf(`[ -f "%s" ] && . "%s"`, source, source))
	}
	text := strings.Join(append(kept, ""), "\n")
	if text == current {
		return
	}
	in.backupCopy(rc)
	must(os.WriteFile(rc, []byte(strings.TrimRight(text, "\n")+"\n"), 0o644))
	if remove {
		in.log("Removed the harness aliases from %s", rc)
	} else {
		in.log("Reconciled harness aliases in %s", rc)
	}
}

// installEngram runs one `engram setup <agent>` per client, exactly as engram's docs/AGENT-SETUP.md documents. The claude-code setup is answered yes to allowlist engram's tools, because declining prompts on every memory call; the codex and opencode setups get a closed stdin, since a blind yes to a future question is not the same answer. Setup registers and pins OpenCode's MCP server and Codex's plugin; Claude Code's MCP is registerEngramClaudeMCP's job.
func (in *installer) installEngram() {
	in.log("Reconciling Engram")
	if !present("engram") {
		die("Engram is missing; run harness install mcps to install it.")
	}
	if in.targets("claude") {
		in.run(strings.NewReader("y\n"), "engram", "setup", "claude-code")
	}
	if in.targets("codex") {
		codex := exec.Command("engram", "setup", "codex")
		codex.Env = models.CodexEnv(in.home)
		codex.Stdout, codex.Stderr = in.stdout, in.stderr
		if err := codex.Run(); err != nil {
			die("engram setup codex failed: %v", err)
		}
	}
	if in.targets("opencode") {
		in.run(nil, "engram", "setup", "opencode")
	}
}

// registerEngramClaudeMCP registers Engram's Claude Code MCP server, which engram's own claude-code setup does not leave anywhere Claude Code reads. Remove then add, so a re-run rewrites the pinned path. README's MCP servers section records why setup is not enough.
func (in *installer) registerEngramClaudeMCP() {
	in.log("Reconciling Engram's Claude Code MCP")
	binary, err := exec.LookPath("engram")
	if err != nil {
		die("Engram is missing; run harness install mcps to install it.")
	}
	if !present("claude") {
		die("Claude Code is missing; run harness install clients --client claude to install it.")
	}
	must(in.registerMCP("claude", "engram", binary))
}

// openCodeConfig reads opencode.json and its mcp object, refusing a file this run could not rewrite. install calls it before any client is touched, so a config this run refuses leaves every registration, Engram's included, as it was.
func (in *installer) openCodeConfig() (string, *object, *object) {
	path := filepath.Join(in.config, "opencode", "opencode.json")
	doc := newObject()
	if data, err := os.ReadFile(path); err == nil {
		if doc, err = parseObject(data); err != nil {
			die("%s: %v", path, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		must(err)
	}
	servers := newObject()
	if current, ok := doc.get("mcp"); ok && string(current) != "null" {
		parsed, err := parseObject(current)
		if err != nil {
			die("%s: mcp is not a JSON object; fix it and re-run.", path)
		}
		servers = parsed
	}
	return path, doc, servers
}

// installAgentBrowser registers agent-browser in every client, since it ships no registrar of its own. Remove then add, so a re-run rewrites the pinned path; OpenCode takes a JSON edit because `opencode mcp add` is interactive.
func (in *installer) installAgentBrowser() {
	in.log("Reconciling agent-browser")
	binary, err := exec.LookPath("agent-browser")
	if err != nil {
		die("agent-browser is missing; run harness install mcps to install it.")
	}
	for _, client := range []string{"codex", "claude", "opencode"} {
		if !in.targets(client) {
			continue
		}
		if client != "opencode" && !present(client) {
			die("%s is missing; run harness install clients --client %s to install it.", clientNames[client], client)
		}
		must(in.registerMCP(client, "agent-browser", binary))
	}
}
