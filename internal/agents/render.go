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

// Render returns {relative output: text}, {relative output: access}, and the report lines.
func Render(configPath string) (map[string]string, map[string]string, []string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, nil, fail(fmt.Sprintf("%s: %v", configPath, err))
	}
	decoded, err := decodeUnique(data)
	if err != nil {
		return nil, nil, nil, fail(fmt.Sprintf("%s: %v", configPath, err))
	}
	config, ok := decoded.(map[string]any)
	if !ok {
		return nil, nil, nil, fail(fmt.Sprintf("%s: config must be an object", configPath))
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

// resolve returns the roles and {(client, role): {field: (value, source)}} for every assigned role, or a failure on the first invalid assignment.
func resolve(config map[string]any) (map[string]any, map[string]map[string][2]string, error) {
	clients, clientsOK := config["clients"].(map[string]any)
	roles, rolesOK := config["roles"].(map[string]any)
	clientNames := make([]string, 0, len(supported))
	for name := range supported {
		clientNames = append(clientNames, name)
	}
	sort.Strings(clientNames)
	if !clientsOK || len(clients) != len(supported) || !containsAll(clients, clientNames) {
		return nil, nil, fail("clients must set exactly " + strings.Join(clientNames, ", "))
	}
	if !rolesOK {
		return nil, nil, fail("roles must be an object")
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
			return nil, nil, fail(fmt.Sprintf("roles.%s: a role name is lowercase letters, digits, and hyphens", repr(name)))
		}
		roleObj, ok := role.(map[string]any)
		if !ok {
			return nil, nil, fail(fmt.Sprintf("roles.%s must be an object", name))
		}
		allowed := []string{"definition", "claude", "opencode"}
		if unknown := unknownKeys(roleObj, allowed); len(unknown) > 0 {
			return nil, nil, fail(fmt.Sprintf("roles.%s: unknown keys %s; a role takes definition, %s", name, strings.Join(unknown, ", "), strings.Join(clientNames, ", ")))
		}
		if def, ok := roleObj["definition"]; ok {
			text, isString := def.(string)
			if !isString || text == "" {
				return nil, nil, fail(fmt.Sprintf("roles.%s.definition must be a path; leave it out for a role with no definition yet", name))
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
					return nil, nil, fail(fmt.Sprintf("clients.%s must set %s; write inherit to use the session's", client, f.name))
				}
				value, source := defBlock[f.name], "client default"
				if v, ok := override[f.name]; ok {
					value, source = v, "role"
				}
				text, isString := value.(string)
				if !isString || !regexp.MustCompile("^(?:"+f.pattern+")$").MatchString(text) {
					return nil, nil, fail(fmt.Sprintf("%s %s %s for %s is unsupported; %s accepts %s", client, f.name, reprValue(value), name, client, strings.ReplaceAll(f.pattern, "|", ", ")))
				}
				settings[f.name] = [2]string{text, source}
			}
			if client == "opencode" && settings["effort"][0] != "inherit" && !strings.HasPrefix(settings["model"][0], "openai/") {
				return nil, nil, fail(fmt.Sprintf("opencode effort for %s is passed as OpenAI's reasoningEffort, so it needs an openai/ model, not %s", name, settings["model"][0]))
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
		return nil, nil, fail(strings.Join(missing, ", ") + " must be assigned with a definition")
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
		return nil, fail(label + " must be an object")
	}
	if unknown := unknownKeys(obj, []string{"model", "effort"}); len(unknown) > 0 {
		return nil, fail(fmt.Sprintf("%s: unknown keys %s; only model and effort are assigned", label, strings.Join(unknown, ", ")))
	}
	return obj, nil
}

var frontmatter = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)$`)

func load(path, name string) (string, string, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", "", fail(fmt.Sprintf("%s: unreadable definition for %s: %v", path, name, err))
	}
	match := frontmatter.FindStringSubmatch(string(raw))
	if match == nil {
		return "", "", "", fail(path + ": missing frontmatter")
	}
	front := map[string]string{}
	for _, line := range strings.Split(match[1], "\n") {
		if key, value, ok := strings.Cut(line, ": "); ok {
			front[key] = value
		}
	}
	body := strings.TrimSpace(match[2])
	if front["name"] != name {
		return "", "", "", fail(fmt.Sprintf("%s: name must be %s, the role it is assigned to", path, name))
	}
	if front["description"] == "" {
		return "", "", "", fail(path + ": missing description")
	}
	level := front["access"]
	expected, ok := required[name]
	if !ok {
		expected = "read-search"
	}
	if level != expected {
		return "", "", "", fail(fmt.Sprintf("%s: access must be %s", path, expected))
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
			return "", "", "", fail(path + ": missing the canonical Access line")
		}
	}
	if body == "" || strings.Contains(body, "'''") {
		return "", "", "", fail(path + ": body must be non-empty and free of '''")
	}
	return front["description"], level, body, nil
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
