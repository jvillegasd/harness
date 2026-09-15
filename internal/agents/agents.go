// Package agents renders every assigned role into one agent definition per client, replacing the Python renderer the installer used to exec.
//
// The assignments file is the one source for roles and their model and effort. `clients` sets each client's default model and effort, and `inherit` there means the session's own choice. Each entry under `roles` names its canonical definition, relative to the assignments file, and may override either field per client. A field resolves from the role's override, else the client default, and generation prints every resolution with its source. A role with no definition is an assignment waiting for its role: it is validated and reported, and nothing is generated for it.
//
// Generation writes <output-dir>/<client>/<role>.md for every defined role and client, removes any other file under <output-dir>, and prints the resolved assignments, the enforcement gaps, and the unverified capabilities. It exits 0 when it wrote or removed something and 3 when the output already matched, so a crash cannot pass for "nothing to do". An invalid or unsupported assignment, an incomplete role, a role asking for other access than its own, or a missing required role exits 4 before anything is written; nothing is substituted.
//
// Output paths and their ancestors must not be symlinks; both generation and checking refuse them before reconciliation.
//
// --check writes nothing. It reports each output that is missing, drifted, unmanaged, or no longer carries its access level's restrictions, and exits 1 when it found any.
package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// adversarial-review launches the reviewers and the refuter and POLICY.md names the executor, so each must exist with exactly this access. Every other role is read-search: only the executor may implement.
var required = map[string]string{
	"executor":       "implement",
	"review-refuter": "read-search",
	"reviewer-a":     "read-search",
	"reviewer-b":     "read-search",
}

// Documented agent frontmatter values: https://code.claude.com/docs/en/sub-agents (model, effort) and https://opencode.ai/docs/agents/ (model, plus provider options such as OpenAI's reasoningEffort). OpenCode passes effort to the provider, so it is accepted only beside an openai/ model.
var supported = map[string][]field{
	"claude":   {{"model", `inherit|sonnet|opus|haiku|fable|claude-[a-z0-9.-]+`}, {"effort", `inherit|low|medium|high|xhigh|max`}},
	"opencode": {{"model", `inherit|[a-z0-9._-]+/[A-Za-z0-9._:/-]+`}, {"effort", `inherit|none|minimal|low|medium|high|xhigh`}},
}

type field struct{ name, pattern string }

const access = "Read and search only. Read and search the repository to follow callers and context. Never edit a file, run a command, fetch from the web, or delegate to another agent, even where the client would let you."

var claudeControls = map[string][]string{
	"read-search": {"tools: Read, Grep, Glob"},
	"implement":   {"disallowedTools: Agent"},
}

// OpenCode appends an agent's rules after its defaults and the last matching rule wins. A reviewer denies every permission and re-allows only reading, keeping the default .env guard a bare `read: allow` would override; whether MCP tools resolve through the same wildcard is unverified. The executor keeps the defaults and denies delegation plus the commit, push, worktree, and forge commands its parent owns.
var opencodeControls = map[string]string{
	"read-search": "permission:\n" +
		"  \"*\": deny\n" +
		"  read:\n" +
		"    \"*\": allow\n" +
		"    \"*.env\": deny\n" +
		"    \"*.env.*\": deny\n" +
		"    \"*.env.example\": allow\n" +
		"  grep: allow\n" +
		"  glob: allow\n" +
		"  list: allow\n",
	"implement": "permission:\n" +
		"  task: deny\n" +
		"  bash:\n" +
		"    \"git commit*\": deny\n" +
		"    \"git push*\": deny\n" +
		"    \"git worktree remove*\": deny\n" +
		"    \"wt remove*\": deny\n" +
		"    \"wt merge*\": deny\n" +
		"    \"wt step commit*\": deny\n" +
		"    \"gh *\": deny\n",
}

// Known from each client's documentation. What no control covers is a gap; what a control should cover but only an installed client can prove is unverified.
var gaps = []string{
	"gap: opencode: a diff file outside the project needs external_directory, which the agent's * deny refuses, so reviewers and the refuter there take the inline fallback",
	"gap: opencode: a standards file outside the project, such as the installed ponytail body, needs the same external_directory, so reviewers there get it inline after their loaded: line omits it",
	"gap: claude: the executor keeps Bash, because a disallowedTools specifier removes the whole tool, so its commit, push, pull request, merge, and tracker limits are instructions only",
	"gap: claude: a model parameter passed at launch overrides an agent's model frontmatter, and an availableModels allowlist substitutes a blocked model with a warning, so the policy forbids the parameter and neither is enforced",
	"gap: opencode: the executor's bash denies match command prefixes, so the same action spelled another way is not denied",
}

var unverified = []string{
	"unverified: every restriction above is static configuration until an installed client is probed at runtime",
	"unverified: claude loading an agent definition through a symlink",
	"unverified: claude reading a diff or standards file outside the project without a permission prompt",
	"unverified: opencode applying the * deny to MCP tools",
	"unverified: the ccyolo and ocyolo launchers leaving agent restrictions in force, since each bypasses or overrides the parent's permissions",
	"unverified: diff delivery's inline fallback and escalation, which are protocol rather than exercised behavior",
	"unverified: claude running a subagent on its model and effort frontmatter, and on the session's effort when effort is absent",
	"unverified: opencode sending an agent's reasoningEffort with its model requests",
	"unverified: claude disallowedTools: Agent and opencode task: deny stopping the executor from delegating",
}

const usageText = `Usage: harness agents [--check] <assignments.json> <output-dir>

Render every assigned role into one agent definition per client. --check reports each output that is missing, drifted, unmanaged, or no longer carries its access level's restrictions, and exits 1 when it found any.
`

// fault carries the exit code a validation failure demands.
type fault struct {
	code int
	msg  string
}

func (e *fault) Error() string { return e.msg }

func fail(msg string, code int) error { return &fault{code: code, msg: msg} }

// Command runs `harness agents` and returns the exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	checking := len(args) > 0 && args[0] == "--check"
	rest := args
	if checking {
		rest = args[1:]
	}
	if len(rest) != 2 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	outputs, accesses, report, err := Render(rest[0])
	if err != nil {
		return report1(err, stderr)
	}
	output := rest[1]
	if code := checkSymlinks(outputs, output, checking, stderr); code != 0 {
		return code
	}
	if checking {
		return Check(outputs, accesses, output, stdout)
	}
	return generate(outputs, output, report, stdout)
}

// GenerateCommand renders and writes the role agents for the installer, refusing a symlinked output path before anything is written.
func GenerateCommand(config, output string, stdout, stderr io.Writer) int {
	outputs, _, report, err := Render(config)
	if err != nil {
		return report1(err, stderr)
	}
	if code := checkSymlinks(outputs, output, false, stderr); code != 0 {
		return code
	}
	return generate(outputs, output, report, stdout)
}

// CheckCommand renders the assignments and reports drift without writing anything, for the installer's verification mode.
func CheckCommand(config, output string, stdout, stderr io.Writer) int {
	outputs, accesses, _, err := Render(config)
	if err != nil {
		return report1(err, stderr)
	}
	if code := checkSymlinks(outputs, output, true, stderr); code != 0 {
		return code
	}
	return Check(outputs, accesses, output, stdout)
}

func report1(err error, w io.Writer) int {
	if f, ok := err.(*fault); ok {
		fmt.Fprintln(w, f.msg)
		return f.code
	}
	fmt.Fprintln(w, err)
	return 4
}

// Render returns {relative output: text}, {relative output: access}, and the report lines.
func Render(configPath string) (map[string]string, map[string]string, []string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, nil, fail(fmt.Sprintf("%s: %v", configPath, err), 4)
	}
	decoded, err := decodeUnique(data)
	if err != nil {
		return nil, nil, nil, fail(fmt.Sprintf("%s: %v", configPath, err), 4)
	}
	config, ok := decoded.(map[string]any)
	if !ok {
		return nil, nil, nil, fail(fmt.Sprintf("%s: config must be an object", configPath), 4)
	}
	roles, resolved, err := resolve(config)
	if err != nil {
		return nil, nil, nil, err
	}
	outputs, accesses, report := map[string]string{}, map[string]string{}, []string{}
	pairs := make([][2]string, 0, len(resolved))
	for key := range resolved {
		client, name, _ := strings.Cut(key, "/")
		pairs = append(pairs, [2]string{client, name})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	for _, pair := range pairs {
		settings := resolved[pair[0]+"/"+pair[1]]
		model, modelSource := settings["model"][0], settings["model"][1]
		effort, effortSource := settings["effort"][0], settings["effort"][1]
		report = append(report, fmt.Sprintf("assignment: %s %s model=%s (%s) effort=%s (%s)", pair[0], pair[1], model, modelSource, effort, effortSource))
		if model != "inherit" {
			report = append(report, fmt.Sprintf("unverified: %s %s model %s is available to this account", pair[0], pair[1], model))
		}
		if effort != "inherit" {
			report = append(report, fmt.Sprintf("unverified: %s %s effort %s is supported by its model", pair[0], pair[1], effort))
		}
	}
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		role := roles[name].(map[string]any)
		def, hasDef := role["definition"]
		if !hasDef {
			report = append(report, fmt.Sprintf("pending: %s has an assignment and no definition, so no agent is generated", name))
			continue
		}
		description, accessLevel, body, err := load(joinPath(filepath.Dir(configPath), def.(string)), name)
		if err != nil {
			return nil, nil, nil, err
		}
		// JSON string quoting is valid YAML for this text.
		quoted := `"` + strings.ReplaceAll(strings.ReplaceAll(description, `\`, `\\`), `"`, `\"`) + `"`
		claude := resolved["claude/"+name]
		opencode := resolved["opencode/"+name]
		// Claude Code documents model: inherit and inherits effort when the field is absent; OpenCode inherits both when absent.
		claudeSettings := "model: " + claude["model"][0] + "\n"
		if claude["effort"][0] != "inherit" {
			claudeSettings += "effort: " + claude["effort"][0] + "\n"
		}
		opencodeSettings := ""
		if opencode["model"][0] != "inherit" {
			opencodeSettings += "model: " + opencode["model"][0] + "\n"
		}
		if opencode["effort"][0] != "inherit" {
			opencodeSettings += "reasoningEffort: " + opencode["effort"][0] + "\n"
		}
		controls := ""
		for _, line := range claudeControls[accessLevel] {
			controls += line + "\n"
		}
		outputs["claude/"+name+".md"] = "---\nname: " + name + "\ndescription: " + quoted + "\n" + controls + claudeSettings + "---\n\n" + body + "\n"
		outputs["opencode/"+name+".md"] = "---\ndescription: " + quoted + "\nmode: subagent\n" + opencodeControls[accessLevel] + opencodeSettings + "---\n\n" + body + "\n"
		accesses["claude/"+name+".md"] = accessLevel
		accesses["opencode/"+name+".md"] = accessLevel
	}
	return outputs, accesses, report, nil
}

// resolve returns the roles and {(client, role): {field: (value, source)}} for every assigned role, or an exit-4 fault on the first invalid assignment.
func resolve(config map[string]any) (map[string]any, map[string]map[string][2]string, error) {
	clients, clientsOK := config["clients"].(map[string]any)
	roles, rolesOK := config["roles"].(map[string]any)
	clientNames := make([]string, 0, len(supported))
	for name := range supported {
		clientNames = append(clientNames, name)
	}
	sort.Strings(clientNames)
	if !clientsOK || len(clients) != len(supported) || !containsAll(clients, clientNames) {
		return nil, nil, fail("clients must set exactly "+strings.Join(clientNames, ", "), 4)
	}
	if !rolesOK {
		return nil, nil, fail("roles must be an object", 4)
	}
	resolved := map[string]map[string][2]string{}
	roleNames := make([]string, 0, len(roles))
	for name := range roles {
		roleNames = append(roleNames, name)
	}
	sort.Strings(roleNames)
	for _, name := range roleNames {
		role := roles[name]
		// The name becomes an output path, so nothing but a plain file name gets through.
		if !roleName.MatchString(name) {
			return nil, nil, fail(fmt.Sprintf("roles.%s: a role name is lowercase letters, digits, and hyphens", repr(name)), 4)
		}
		roleObj, ok := role.(map[string]any)
		if !ok {
			return nil, nil, fail(fmt.Sprintf("roles.%s must be an object", name), 4)
		}
		allowed := []string{"definition", "claude", "opencode"}
		if unknown := unknownKeys(roleObj, allowed); len(unknown) > 0 {
			return nil, nil, fail(fmt.Sprintf("roles.%s: unknown keys %s; a role takes definition, %s", name, strings.Join(unknown, ", "), strings.Join(clientNames, ", ")), 4)
		}
		if def, ok := roleObj["definition"]; ok {
			text, isString := def.(string)
			if !isString || text == "" {
				return nil, nil, fail(fmt.Sprintf("roles.%s.definition must be a path; leave it out for a role with no definition yet", name), 4)
			}
		}
		for _, client := range clientNames {
			defBlock, err := assignmentBlock(clients[client], "clients."+client)
			if err != nil {
				return nil, nil, err
			}
			var override map[string]any
			if raw, ok := roleObj[client]; ok {
				override, err = assignmentBlock(raw, fmt.Sprintf("roles.%s.%s", name, client))
				if err != nil {
					return nil, nil, err
				}
			} else {
				override = map[string]any{}
			}
			settings := map[string][2]string{}
			for _, f := range supported[client] {
				if _, ok := defBlock[f.name]; !ok {
					return nil, nil, fail(fmt.Sprintf("clients.%s must set %s; write inherit to use the session's", client, f.name), 4)
				}
				value, source := defBlock[f.name], "client default"
				if v, ok := override[f.name]; ok {
					value, source = v, "role"
				}
				text, isString := value.(string)
				if !isString || !regexp.MustCompile("^(?:"+f.pattern+")$").MatchString(text) {
					return nil, nil, fail(fmt.Sprintf("%s %s %s for %s is unsupported; %s accepts %s", client, f.name, reprValue(value), name, client, strings.ReplaceAll(f.pattern, "|", ", ")), 4)
				}
				settings[f.name] = [2]string{text, source}
			}
			if client == "opencode" && settings["effort"][0] != "inherit" && !strings.HasPrefix(settings["model"][0], "openai/") {
				return nil, nil, fail(fmt.Sprintf("opencode effort for %s is passed as OpenAI's reasoningEffort, so it needs an openai/ model, not %s", name, settings["model"][0]), 4)
			}
			resolved[client+"/"+name] = settings
		}
	}
	requiredNames := make([]string, 0, len(required))
	for name := range required {
		requiredNames = append(requiredNames, name)
	}
	sort.Strings(requiredNames)
	var missing []string
	for _, name := range requiredNames {
		role, ok := roles[name].(map[string]any)
		if !ok {
			missing = append(missing, name)
			continue
		}
		if _, ok := role["definition"]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, nil, fail(strings.Join(missing, ", ")+" must be assigned with a definition", 4)
	}
	return roles, resolved, nil
}

var roleName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func containsAll(m map[string]any, keys []string) bool {
	for _, key := range keys {
		if _, ok := m[key]; !ok {
			return false
		}
	}
	return true
}

func unknownKeys(m map[string]any, allowed []string) []string {
	set := map[string]bool{}
	for _, key := range allowed {
		set[key] = true
	}
	var unknown []string
	for key := range m {
		if !set[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown
}

func assignmentBlock(block any, label string) (map[string]any, error) {
	obj, ok := block.(map[string]any)
	if !ok {
		return nil, fail(label+" must be an object", 4)
	}
	if unknown := unknownKeys(obj, []string{"model", "effort"}); len(unknown) > 0 {
		return nil, fail(fmt.Sprintf("%s: unknown keys %s; only model and effort are assigned", label, strings.Join(unknown, ", ")), 4)
	}
	return obj, nil
}

var frontmatter = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)$`)

func load(path, name string) (string, string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", "", fail(fmt.Sprintf("%s: unreadable definition for %s: %v", path, name, err), 4)
	}
	match := frontmatter.FindStringSubmatch(string(raw))
	if match == nil {
		return "", "", "", fail(path+": missing frontmatter", 4)
	}
	front := map[string]string{}
	for _, line := range strings.Split(match[1], "\n") {
		if key, value, ok := strings.Cut(line, ": "); ok {
			front[key] = value
		}
	}
	body := strings.TrimSpace(match[2])
	if front["name"] != name {
		return "", "", "", fail(fmt.Sprintf("%s: name must be %s, the role it is assigned to", path, name), 4)
	}
	if front["description"] == "" {
		return "", "", "", fail(path+": missing description", 4)
	}
	level := front["access"]
	expected, ok := required[name]
	if !ok {
		expected = "read-search"
	}
	if level != expected {
		return "", "", "", fail(fmt.Sprintf("%s: access must be %s", path, expected), 4)
	}
	if level == "read-search" {
		found := false
		for _, line := range splitLines(body) {
			if line == access {
				found = true
				break
			}
		}
		if !found {
			return "", "", "", fail(path+": missing the canonical Access line", 4)
		}
	}
	if body == "" || strings.Contains(body, "'''") {
		return "", "", "", fail(path+": body must be non-empty and free of '''", 4)
	}
	return front["description"], level, body, nil
}

func restricted(relative, text, level string) bool {
	lines := splitLines(text)
	if strings.HasPrefix(relative, "claude/") {
		head := lines
		if countExact(lines, "---") >= 2 {
			index := -1
			for i := 1; i < len(lines); i++ {
				if lines[i] == "---" {
					index = i
					break
				}
			}
			if index >= 0 {
				head = lines[:index]
			}
		}
		var kept []string
		for _, line := range head {
			if strings.HasPrefix(line, "tools:") || strings.HasPrefix(line, "disallowedTools:") {
				kept = append(kept, line)
			}
		}
		want := claudeControls[level]
		if len(kept) != len(want) {
			return false
		}
		for i := range kept {
			if kept[i] != want[i] {
				return false
			}
		}
		return true
	}
	head := ""
	if strings.HasPrefix(text, "---\n") {
		parts := strings.Split(text, "---\n")
		if len(parts) > 1 {
			head = parts[1]
		}
	}
	kept := ""
	for _, line := range splitLines(head) {
		if strings.HasPrefix(line, "description:") || strings.HasPrefix(line, "model:") || strings.HasPrefix(line, "reasoningEffort:") {
			continue
		}
		kept += line + "\n"
	}
	return kept == "mode: subagent\n"+opencodeControls[level]
}

// splitLines matches Python's str.splitlines: it drops the empty tail a trailing newline would otherwise leave.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func countExact(lines []string, want string) int {
	count := 0
	for _, line := range lines {
		if line == want {
			count++
		}
	}
	return count
}

// joinPath mirrors Python's Path / part: an absolute part replaces the base instead of being appended.
func joinPath(base, part string) string {
	if filepath.IsAbs(part) {
		return part
	}
	return filepath.Join(base, part)
}

func existing(output string) map[string]string {
	found := map[string]string{}
	info, err := os.Stat(output)
	if err != nil || !info.IsDir() {
		return found
	}
	_ = filepath.WalkDir(output, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(output, path)
		if err != nil {
			return nil
		}
		found[filepath.ToSlash(rel)] = path
		return nil
	})
	return found
}

func checkSymlinks(outputs map[string]string, output string, checking bool, stderr io.Writer) int {
	code := 4
	if checking {
		code = 1
	}
	names := make([]string, 0, len(outputs))
	for relative := range outputs {
		names = append(names, relative)
	}
	sort.Strings(names)
	for _, relative := range names {
		path := filepath.Join(output, filepath.FromSlash(relative))
		entries := append([]string{path}, parents(path)...)
		for _, entry := range entries {
			if info, err := os.Lstat(entry); err == nil && info.Mode()&os.ModeSymlink != 0 {
				fmt.Fprintf(stderr, "%s: symlink in role agent output path\n", entry)
				return code
			}
		}
	}
	return 0
}

// parents lists path's ancestors from its parent up to the filesystem root.
func parents(path string) []string {
	var out []string
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		out = append(out, dir)
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return out
}

// Check reports each output that is missing, drifted, unmanaged, or widened, returning 1 when it found any.
func Check(outputs, accesses map[string]string, output string, stdout io.Writer) int {
	present := existing(output)
	var findings []string
	for relative := range present {
		if _, ok := outputs[relative]; !ok {
			findings = append(findings, "unmanaged: "+relative)
		}
	}
	sort.Strings(findings)
	names := make([]string, 0, len(outputs))
	for relative := range outputs {
		names = append(names, relative)
	}
	sort.Strings(names)
	for _, relative := range names {
		expected := outputs[relative]
		path, ok := present[relative]
		if !ok {
			findings = append(findings, "missing: "+relative)
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			findings = append(findings, "missing: "+relative)
			continue
		}
		if !restricted(relative, string(raw), accesses[relative]) {
			findings = append(findings, "widened: "+relative)
		} else if string(raw) != expected {
			findings = append(findings, "changed: "+relative)
		}
	}
	if len(findings) > 0 {
		fmt.Fprintln(stdout, strings.Join(findings, "\n"))
		return 1
	}
	return 0
}

func generate(outputs map[string]string, output string, report []string, stdout io.Writer) int {
	changed := false
	for relative, path := range existing(output) {
		if _, ok := outputs[relative]; !ok {
			_ = os.Remove(path)
			changed = true
		}
	}
	names := make([]string, 0, len(outputs))
	for relative := range outputs {
		names = append(names, relative)
	}
	sort.Strings(names)
	for _, relative := range names {
		text := outputs[relative]
		path := filepath.Join(output, filepath.FromSlash(relative))
		if raw, err := os.ReadFile(path); err == nil && string(raw) == text {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintln(stdout, err)
			return 4
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			fmt.Fprintln(stdout, err)
			return 4
		}
		changed = true
	}
	lines := append(append(append([]string{}, report...), gaps...), unverified...)
	fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	if changed {
		return 0
	}
	return 3
}

func repr(s string) string { return "'" + s + "'" }
func reprValue(v any) string {
	switch value := v.(type) {
	case string:
		return repr(value)
	case nil:
		return "None"
	default:
		return fmt.Sprint(value)
	}
}

// decodeUnique parses JSON, refusing a duplicate key anywhere, which encoding/json would otherwise resolve silently.
func decodeUnique(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after the JSON document")
	}
	return value, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		obj := map[string]any{}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("object key is not a string")
			}
			if _, exists := obj[key]; exists {
				return nil, fmt.Errorf("duplicate keys: %s", key)
			}
			value, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			obj[key] = value
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return obj, nil
	case '[':
		var arr []any
		for dec.More() {
			value, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, value)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return arr, nil
	}
	return nil, fmt.Errorf("unexpected delimiter %q", delim)
}
