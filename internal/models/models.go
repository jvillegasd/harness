// Package models chooses, per machine, which client, model, and effort each delegated role runs on. The choice lives in the models block of ~/.harness/state.json: a preset, plus per-role overrides. The installer renders it into each client's role agents, and a workflow asks it which client and model a role carries before launching it through Orca.
package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/agents"
)

// Assignment is one role's client, model, and effort. An empty Effort inherits the session's.
type Assignment struct {
	Client string `json:"client"`
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
}

// Roles are the delegated roles a preset assigns, in report order.
var Roles = []string{"executor", "reviewer-a", "reviewer-b", "review-refuter"}

const defaultPreset = "balanced"

func claude(model, effort string) Assignment { return Assignment{"claude", model, effort} }
func opencode(model string) Assignment       { return Assignment{"opencode", model, ""} }

// Presets are the named assignment tables. balanced sends implementation to a cheaper OpenCode model and keeps review on the session's Claude model, because a reviewer too weak to find the bug is no saving.
var Presets = map[string]map[string]Assignment{
	"balanced": {
		"executor": opencode("opencode-go/glm-5.3"), "reviewer-a": claude("inherit", ""), "reviewer-b": claude("inherit", ""), "review-refuter": claude("inherit", ""),
	},
	"performance": {
		"executor": claude("opus", "high"), "reviewer-a": claude("fable", "high"), "reviewer-b": claude("fable", "high"), "review-refuter": claude("opus", "high"),
	},
	"economy": {
		"executor": opencode("opencode-go/deepseek-v4-flash"), "reviewer-a": claude("sonnet", ""), "reviewer-b": claude("sonnet", ""), "review-refuter": claude("sonnet", ""),
	},
	"diversity": {
		"executor": claude("sonnet", ""), "reviewer-a": claude("inherit", ""), "reviewer-b": opencode("openai/gpt-5.6-sol"), "review-refuter": opencode("opencode-go/glm-5.3"),
	},
}

// claudeEfforts lists each Claude alias's supported efforts, from Claude Code's effort documentation; "" inherits.
var claudeEfforts = map[string][]string{
	"inherit": {""},
	"fable":   {"", "low", "medium", "high", "xhigh", "max"},
	"opus":    {"", "low", "medium", "high", "xhigh", "max"},
	"sonnet":  {"", "low", "medium", "high", "max"},
	"haiku":   {""},
}

// openaiEfforts are what OpenCode passes as OpenAI's reasoningEffort, the only effort it sends.
var openaiEfforts = []string{"", "none", "minimal", "low", "medium", "high", "xhigh"}

// Validate rejects an assignment the client cannot run: a malformed one, or an OpenCode id `opencode models` does not list on this machine.
func Validate(a Assignment) error {
	if err := shape(a); err != nil {
		return err
	}
	if a.Client == "opencode" {
		listed, err := OpenCodeModels()
		if err != nil {
			return err
		}
		if !slices.Contains(listed, a.Model) {
			return fmt.Errorf("opencode has no model %q; run `opencode models`", a.Model)
		}
	}
	return nil
}

// OpenCodeModels lists the provider/model ids OpenCode can run on this machine.
func OpenCodeModels() ([]string, error) {
	out, err := exec.Command("opencode", "models").Output()
	if err != nil {
		return nil, fmt.Errorf("listing OpenCode models failed: %v", err)
	}
	return strings.Fields(string(out)), nil
}

// shape rejects what no machine can run: an unknown client or Claude alias, or an effort the model does not support.
func shape(a Assignment) error {
	switch a.Client {
	case "claude":
		efforts, ok := claudeEfforts[a.Model]
		if !ok {
			return fmt.Errorf("unknown Claude model %q; choose inherit, fable, opus, sonnet, or haiku", a.Model)
		}
		if !slices.Contains(efforts, a.Effort) {
			return fmt.Errorf("claude %s does not support effort %q", a.Model, a.Effort)
		}
	case "opencode":
		if !strings.Contains(a.Model, "/") {
			return fmt.Errorf("opencode model %q is not a provider/model id", a.Model)
		}
		if a.Effort != "" && (!strings.HasPrefix(a.Model, "openai/") || !slices.Contains(openaiEfforts, a.Effort)) {
			return fmt.Errorf("opencode sends effort only as OpenAI's reasoningEffort, so %q is unsupported for %s", a.Effort, a.Model)
		}
	default:
		return fmt.Errorf("unknown client %q; choose claude or opencode", a.Client)
	}
	return nil
}

// Choice is the stored models block.
type Choice struct {
	Preset string                `json:"preset,omitempty"`
	Roles  map[string]Assignment `json:"roles,omitempty"`
}

// Resolved is one role's effective assignment and where it came from: "override" or "preset:<name>".
type Resolved struct {
	Assignment
	Source string
}

// Resolve applies per-role overrides over the chosen preset, which defaults to balanced.
func (c Choice) Resolve() map[string]Resolved {
	preset := c.Preset
	if preset == "" {
		preset = defaultPreset
	}
	out := map[string]Resolved{}
	for _, role := range Roles {
		if a, ok := c.Roles[role]; ok {
			out[role] = Resolved{a, "override"}
			continue
		}
		out[role] = Resolved{Presets[preset][role], "preset:" + preset}
	}
	return out
}

// ForRender turns the resolution into what the role-agent renderer takes.
func (c Choice) ForRender() map[string]agents.Assigned {
	out := map[string]agents.Assigned{}
	for role, r := range c.Resolve() {
		out[role] = agents.Assigned{Client: r.Client, Model: r.Model, Effort: r.Effort, Source: r.Source}
	}
	return out
}

// StatePath is ~/.harness/state.json under home, which the installer shares.
func StatePath(home string) string { return filepath.Join(home, ".harness", "state.json") }

// Load reads the models block, treating a missing file or block as no choice.
func Load(path string) (Choice, error) {
	doc, err := readDoc(path)
	if err != nil {
		return Choice{}, err
	}
	var c Choice
	raw, ok := doc["models"]
	if !ok {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return Choice{}, fmt.Errorf("%s: models: %w; run `harness models reset`", path, err)
	}
	if _, ok := Presets[c.Preset]; c.Preset != "" && !ok {
		return Choice{}, fmt.Errorf("%s: unknown preset %q; run `harness models reset`", path, c.Preset)
	}
	for role, a := range c.Roles {
		if err := knownRole(role); err != nil {
			return Choice{}, fmt.Errorf("%s: %v; run `harness models reset`", path, err)
		}
		if err := shape(a); err != nil {
			return Choice{}, fmt.Errorf("%s: %s: %v; run `harness models reset %s`", path, role, err, role)
		}
	}
	return c, nil
}

// Save writes the models block and keeps every other key, removing the block when the choice is empty.
func Save(path string, c Choice) error {
	doc, err := readDoc(path)
	if err != nil {
		return err
	}
	if c.Preset == "" && len(c.Roles) == 0 {
		delete(doc, "models")
	} else {
		raw, err := json.Marshal(c)
		if err != nil {
			return err
		}
		doc["models"] = raw
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return replace(path, append(data, '\n'))
}

// replace writes data to a fresh temporary file beside path and renames it over path, so a reader never sees half a file.
func replace(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "state-*.json")
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(f.Name(), 0o644)
	}
	if werr != nil {
		_ = os.Remove(f.Name())
		return werr
	}
	return os.Rename(f.Name(), path)
}

// Replace writes path through a temporary file and a rename; the installer uses it for the same state file.
func Replace(path string, data []byte) error { return replace(path, data) }

func readDoc(path string) (map[string]json.RawMessage, error) {
	doc := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}
