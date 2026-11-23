package agents

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

type Assigned struct {
	Client, Model, Effort, Source string
}

func Render(root string, assigned map[string]Assigned) (map[string]string, map[string]string, []string, error) {
	resolved, err := resolve(assigned)
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
	for _, role := range Roles {
		name := role.Name
		description, accessLevel, body, err := load(filepath.Join(root, filepath.FromSlash(role.Definition)), role)
		if err != nil {
			return nil, nil, nil, err
		}

		quoted := `"` + strings.ReplaceAll(strings.ReplaceAll(description, `\`, `\\`), `"`, `\"`) + `"`
		claude := resolved["claude/"+name]
		codex := resolved["codex/"+name]
		opencode := resolved["opencode/"+name]

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

		outputs["opencode/"+name+".md"] = "---\ndescription: " + quoted + "\nmode: all\n" + opencodeControls[accessLevel] + opencodeSettings + "---\n\n" + body + "\n"

		codexSettings := ""
		if codex["model"][0] != "inherit" {
			codexSettings += "model = \"" + codex["model"][0] + "\"\n"
		}
		if codex["effort"][0] != "inherit" {
			codexSettings += "model_reasoning_effort = \"" + codex["effort"][0] + "\"\n"
		}
		outputs["codex/"+name+".toml"] = "name = \"" + name + "\"\ndescription = " + quoted + "\n" + codexSettings + codexInstructions + body + codexClose + codexTail(accessLevel)
		accesses["claude/"+name+".md"] = accessLevel
		accesses["codex/"+name+".toml"] = accessLevel
		accesses["opencode/"+name+".md"] = accessLevel
	}
	return outputs, accesses, report, nil
}

func resolve(assigned map[string]Assigned) (map[string]map[string][2]string, error) {
	clients := slices.Sorted(maps.Keys(supported))
	resolved := map[string]map[string][2]string{}
	for _, role := range Roles {
		for _, client := range clients {
			settings := map[string][2]string{"model": {"inherit", "client default"}, "effort": {"inherit", "client default"}}
			if a, ok := assigned[role.Name]; ok && a.Client == client {
				effort := a.Effort
				if effort == "" {
					effort = "inherit"
				}
				settings = map[string][2]string{"model": {a.Model, a.Source}, "effort": {effort, a.Source}}
			}
			for _, f := range supported[client] {
				if value := settings[f.name][0]; !regexp.MustCompile("^(?:" + f.pattern + ")$").MatchString(value) {
					return nil, fail(fmt.Sprintf("%s %s %s for %s is unsupported; %s accepts %s", client, f.name, repr(value), role.Name, client, strings.ReplaceAll(f.pattern, "|", ", ")))
				}
			}
			if client == "opencode" && settings["effort"][0] != "inherit" && !strings.HasPrefix(settings["model"][0], "openai/") {
				return nil, fail(fmt.Sprintf("opencode effort for %s is passed as OpenAI's reasoningEffort, so it needs an openai/ model, not %s", role.Name, settings["model"][0]))
			}
			resolved[client+"/"+role.Name] = settings
		}
	}
	return resolved, nil
}

func extendedBody(path, level string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	match := frontmatter.FindStringSubmatch(string(raw))
	if match == nil {
		return "", fmt.Errorf("missing frontmatter")
	}
	front := map[string]string{}
	for _, line := range strings.Split(match[1], "\n") {
		if key, value, ok := strings.Cut(line, ": "); ok {
			front[key] = value
		}
	}
	if front["access"] != level {
		return "", fmt.Errorf("access must be %s, the access of the definition extending it", level)
	}
	if front["extends"] != "" {
		return "", fmt.Errorf("it extends another definition itself")
	}
	return strings.TrimSpace(match[2]), nil
}

var frontmatter = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)$`)

func load(path string, role Role) (string, string, string, error) {
	name := role.Name
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
	if level != role.Access {
		return "", "", "", fail(fmt.Sprintf("%s: access must be %s", path, role.Access))
	}

	base := front["extends"]
	if base != "" {
		baseBody, err := extendedBody(filepath.Join(filepath.Dir(path), base), level)
		if err != nil {
			return "", "", "", fail(fmt.Sprintf("%s: extends %s: %v", path, base, err))
		}
		body = baseBody + "\n\n" + body
	}
	if level == "read-search" {
		if !slices.Contains(splitLines(body), access) {
			if base != "" {
				return "", "", "", fail(fmt.Sprintf("%s: missing the canonical Access line, which its base %s carries", path, base))
			}
			return "", "", "", fail(path + ": missing the canonical Access line")
		}
	}
	if body == "" || strings.Contains(body, "'''") {
		return "", "", "", fail(path + ": body must be non-empty and free of '''")
	}

	if !tomlText(front["description"], false) || !tomlText(body, true) {
		return "", "", "", fail(path + ": description and body must be UTF-8 without control characters other than tab and line feed")
	}
	return front["description"], level, body, nil
}

func tomlText(s string, multiline bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if (r < 0x20 && r != '\t' && (r != '\n' || !multiline)) || r == 0x7f {
			return false
		}
	}
	return true
}

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

func repr(s string) string { return "'" + s + "'" }
