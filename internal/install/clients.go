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

type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func newObject() *object {
	return &object{vals: map[string]json.RawMessage{}}
}

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

func encode(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	must(enc.Encode(v))
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

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

func (in *installer) writeIfChanged(path string, before, after []byte, message string) {
	if same(before, after) {
		return
	}
	in.backupCopy(path)
	must(os.WriteFile(path, after, 0o644))
	in.log(message, path)
}

const harnessCommand = `.local/bin/harness"`

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
		if key == "skillOverrides" {
			current, _ := live.get(key)
			value = mergeEntries(current, value, true)
		}
		live.set(key, value)
	}
	in.writeIfChanged(path, before, render(live), "Reconciled the tracked Claude settings in %s")
}

func mergeEntries(live, tracked json.RawMessage, add bool) json.RawMessage {
	merged, err := parseObject(live)
	if err != nil {
		merged = newObject()
	}
	want, err := parseObject(tracked)
	must(err)
	for _, name := range want.keys {
		have, ok := merged.get(name)
		switch {
		case add && !ok:
			merged.set(name, want.vals[name])
		case !add && ok && string(have) == string(want.vals[name]):
			merged.del(name)
		}
	}
	return encode(merged)
}

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
		if ok && key == "skillOverrides" {
			if rest := mergeEntries(current, tracked.vals[key], false); string(rest) != "{}" {
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
		if key == "skillOverrides" {
			owned, _ := live[key].(map[string]any)
			for name, entry := range want.(map[string]any) {
				value, present := owned[name]
				if removed && reflect.DeepEqual(value, entry) || !removed && !present {
					return false
				}
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

func (in *installer) reconcileCodexHooks(remove bool) {
	path := in.codexHooksPath()
	if remove && !isFile(path) {
		return
	}
	live, data := readObject(path, "the harness compaction hook cannot be merged into it")
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

func (in *installer) reconcileOpenCodePermission(remove bool) {
	path := in.openCodeConfigPath()
	if remove && !isFile(path) {
		in.recordOpenCodeDefaults(nil)
		return
	}
	live, data := readObject(path, "the harness permission rules cannot be merged into it")
	current, _ := live.get("permission")
	merged, added, err := mergePermission(current, in.trackedOpenCodePermission(), !remove, in.loadState().OpenCodeDefaults)
	if err != nil {
		die("%s: %v; fix it and re-run.", path, err)
	}
	if merged == nil {
		live.del("permission")
	} else {
		live.set("permission", merged)
	}

	before, _ := parseObject(data)
	after := render(live)
	if before == nil || !bytes.Equal(render(before), after) {
		must(os.MkdirAll(filepath.Dir(path), 0o755))
		in.backupCopy(path)
		must(os.WriteFile(path, after, 0o644))
		in.log("Reconciled the harness permission rules in %s", path)
	}
	in.recordOpenCodeDefaults(added)
}

func (in *installer) recordOpenCodeDefaults(patterns []string) {
	if !slices.Equal(in.loadState().OpenCodeDefaults, patterns) {
		in.updateState(func(s *state) { s.OpenCodeDefaults = patterns })
	}
}

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

func strictness(raw json.RawMessage) int {
	var action string
	_ = json.Unmarshal(raw, &action)
	return slices.Index([]string{"allow", "ask", "deny"}, action)
}

// A catch-all appended after the user's rules would override every one of them,
// so this class holds defaults: they go first and any rule of the user's wins.
// The patterns the install added are recorded, because a user's rule can equal a tracked one.
const defaultsClass = "external_directory"

func hasCatchAll(permission, rules *object) bool {
	if _, ok := rules.get("*"); ok {
		return true
	}
	all, ok := permission.get("*")
	return ok && (same(all, encode("ask")) || same(all, encode("deny")))
}

func mergePermission(live json.RawMessage, tracked *object, add bool, added []string) (json.RawMessage, []string, error) {
	permission, err := actionRules(live)
	if err != nil {
		return nil, nil, errors.New("permission is neither an action nor a JSON object")
	}
	for _, class := range tracked.keys {
		ours, err := parseObject(tracked.vals[class])
		must(err)
		rules, err := actionRules(permission.vals[class])
		if err != nil {
			return nil, nil, fmt.Errorf("permission.%s is neither an action nor a JSON object", class)
		}
		if class == defaultsClass {
			if add {
				if hasCatchAll(permission, rules) {
					continue
				}
				merged := newObject()
				for _, pattern := range ours.keys {
					if _, had := rules.get(pattern); !had {
						merged.set(pattern, ours.vals[pattern])
						if !slices.Contains(added, pattern) {
							added = append(added, pattern)
						}
					}
				}
				for _, pattern := range rules.keys {
					merged.set(pattern, rules.vals[pattern])
				}
				permission.set(class, encode(merged))
				continue
			}
			removed := false
			for _, pattern := range added {
				if have, ok := rules.get(pattern); ok && same(have, ours.vals[pattern]) {
					rules.del(pattern)
					removed = true
				}
			}
			if !removed {
				continue
			}
			if len(rules.keys) == 0 {
				permission.del(class)
			} else {
				permission.set(class, encode(rules))
			}
			continue
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
	if !add {
		added = nil
	}
	if len(permission.keys) == 0 {
		return nil, added, nil
	}
	return encode(permission), added, nil
}

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
		if class == defaultsClass {
			// The uninstall has cleared the record by now, so a rule left here is the user's.
			if !removed && !hasCatchAll(permission, rules) {
				return false
			}
			continue
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

func (in *installer) aliasRCs() []string {
	var out []string
	for _, name := range []string{".zshrc", ".bashrc", ".bash_profile", ".bash_login", ".profile"} {
		out = append(out, filepath.Join(in.home, name))
	}
	return out
}

func (in *installer) aliasRC() string {
	shell := filepath.Base(os.Getenv("SHELL"))
	if os.Getenv("SHELL") == "" {
		shell = "bash"
		if runtime.GOOS == "darwin" {
			shell = "zsh"
		}
	}
	switch shell {
	case "zsh":
		return filepath.Join(in.home, ".zshrc")
	case "bash":
		if runtime.GOOS != "darwin" {
			return filepath.Join(in.home, ".bashrc")
		}

		for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
			if path := filepath.Join(in.home, name); isFile(path) {
				return path
			}
		}
		return filepath.Join(in.home, ".bash_profile")
	}
	return ""
}

func (in *installer) reconcileAliases(remove bool) {
	if remove {

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

	lines := lineBreak.Split(current, -1)
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var kept []string
	dropped := false
	for i, line := range lines {
		alias := aliasLine.FindStringSubmatch(line)
		managed := alias != nil && (alias[1] == "ccyolo" || alias[1] == "cxyolo" || alias[1] == "oyolo" || alias[1] == "ocyolo")

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

func readObject(path, message string) (*object, []byte) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		data = []byte("{}\n")
	} else if err != nil {
		die("Reading %s failed: %v", path, err)
	}
	live, err := parseObject(data)
	if err != nil {
		die("%s is not a JSON object, so "+message+": %v", path, err)
	}
	return live, data
}
