// Package models chooses, per machine, which client, model, and effort each delegated role runs on. The choice lives in the models block of ~/.harness/state.json: a preset, plus per-role overrides. The installer renders it into each client's role agents, and a workflow asks it which client a role carries: its own client launches the role as a native subagent, and another client goes through Orca.
package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/agents"
	"github.com/jvillegasd/harness/internal/statefile"
)

// Assignment is one role's client, model, and effort. An empty Effort inherits the session's.
type Assignment struct {
	Client string `json:"client"`
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
}

// Roles are the delegated roles a preset assigns, in report order.
var Roles = []string{"executor", "code-search", "reviewer"}

// retired roles were merged into reviewer or removed; an override left for one is dropped on load rather than failing every install.
var retired = map[string]bool{"reviewer-a": true, "reviewer-b": true, "review-refuter": true}

const defaultPreset = "balanced"

func claude(model, effort string) Assignment { return Assignment{"claude", model, effort} }
func codex(model, effort string) Assignment  { return Assignment{"codex", model, effort} }

// Presets are the named assignment tables. balanced sends implementation to a cheaper Codex model and code search to Sonnet, whose citations the parent re-reads, and pins review to Opus at high effort, because a reviewer too weak to find the bug is no saving and inheriting the session would tie review strength to whatever the session runs. OpenCode stays available through `harness models set`.
var Presets = map[string]map[string]Assignment{
	"balanced": {
		"executor": codex("gpt-6-luna", ""), "code-search": claude("sonnet", ""), "reviewer": claude("opus", "high"),
	},
	"performance": {
		"executor": claude("opus", "high"), "code-search": claude("fable", "high"), "reviewer": claude("fable", "high"),
	},
	"economy": {
		"executor": codex("gpt-5.6-luna", ""), "code-search": claude("haiku", ""), "reviewer": claude("sonnet", ""),
	},
	"diversity": {
		"executor": claude("sonnet", ""), "code-search": claude("inherit", ""), "reviewer": codex("gpt-6.1-sol", "high"),
	},
}

// claudeEfforts lists each Claude alias's supported efforts, from Claude Code's effort documentation; "" inherits.
var claudeEfforts = map[string][]string{
	"inherit": {""},
	"fable":   {"", "low", "medium", "high", "xhigh", "max"},
	"opus":    {"", "low", "medium", "high", "xhigh", "max"},
	"sonnet":  {"", "low", "medium", "high", "xhigh", "max"},
	"haiku":   {""},
}

// openaiEfforts are what OpenCode passes as OpenAI's reasoningEffort, the only effort it sends.
var openaiEfforts = []string{"", "none", "minimal", "low", "medium", "high", "xhigh"}

// codexModel is the slug shape the role-agent renderer accepts, inherit included.
var codexModel = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// codexEfforts are the model_reasoning_effort values Codex's catalog uses; which of them a model supports is the catalog's to say.
var codexEfforts = []string{"", "low", "medium", "high", "xhigh", "max", "ultra"}

// Validate rejects an assignment the client cannot run: a malformed one, or a model this machine's OpenCode or Codex does not list.
func Validate(a Assignment) error {
	if err := shape(a); err != nil {
		return err
	}
	return new(Catalog).Runnable(a)
}

// ErrNotListed is a model the client's own listing leaves out, as opposed to a listing that failed.
var ErrNotListed = errors.New("not listed")

// Catalog is what the non-Claude clients can run on this machine, each client asked once and only when an assignment needs it.
type Catalog struct {
	opencode, codex         []string
	codexEfforts            map[string][]string
	opencodeErr, codexErr   error
	opencodeDone, codexDone bool
}

// Runnable reports whether this machine can run a well-formed assignment: nil, an error wrapping ErrNotListed, or the listing's own failure. A Claude alias and Codex's inherit always run.
func (c *Catalog) Runnable(a Assignment) error {
	switch a.Client {
	case "opencode":
		listed, err := c.OpenCode()
		if err != nil {
			return err
		}
		if !slices.Contains(listed, a.Model) {
			return fmt.Errorf("opencode has no model %q; run `opencode models`: %w", a.Model, ErrNotListed)
		}
	case "codex":
		if a.Model == "inherit" {
			return nil
		}
		listed, err := c.Codex()
		if err != nil {
			return err
		}
		if !slices.Contains(listed, a.Model) {
			return fmt.Errorf("codex has no model %q; run `codex debug models`: %w", a.Model, ErrNotListed)
		}
		if a.Effort != "" && !slices.Contains(c.codexEfforts[a.Model], a.Effort) {
			return fmt.Errorf("codex %s does not support effort %q: %w", a.Model, a.Effort, ErrNotListed)
		}
	}
	return nil
}

// OpenCode lists the provider/model ids OpenCode can run on this machine.
func (c *Catalog) OpenCode() ([]string, error) {
	if !c.opencodeDone {
		c.opencodeDone = true
		out, err := exec.Command("opencode", "models").Output()
		if err != nil {
			c.opencodeErr = fmt.Errorf("listing OpenCode models failed: %v", err)
		} else {
			c.opencode = strings.Fields(string(out))
		}
	}
	return c.opencode, c.opencodeErr
}

// Codex lists the model slugs Codex offers on this machine, from `codex debug models`; a hidden model is not one to choose.
func (c *Catalog) Codex() ([]string, error) {
	if !c.codexDone {
		c.codexDone = true
		c.codex, c.codexEfforts, c.codexErr = codexCatalog()
	}
	return c.codex, c.codexErr
}

// CodexEfforts lists the efforts a listed Codex model supports, "" first for inherit.
func (c *Catalog) CodexEfforts(model string) []string {
	if _, err := c.Codex(); err != nil || model == "inherit" {
		return []string{""}
	}
	return append([]string{""}, c.codexEfforts[model]...)
}

func codexCatalog() ([]string, map[string][]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	out, err := CodexCommand(home, "debug", "models").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("listing Codex models failed: %v", err)
	}
	var catalog struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
			Levels     []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(out, &catalog); err != nil {
		return nil, nil, fmt.Errorf("reading Codex's model catalog failed: %v", err)
	}
	var slugs []string
	efforts := map[string][]string{}
	for _, m := range catalog.Models {
		if m.Visibility != "list" {
			continue
		}
		slugs = append(slugs, m.Slug)
		for _, level := range m.Levels {
			efforts[m.Slug] = append(efforts[m.Slug], level.Effort)
		}
	}
	return slugs, efforts, nil
}

// CodexCommand runs codex against ~/.codex under home, whatever CODEX_HOME the caller's terminal set, because the harness installs Codex's configuration there and Orca points CODEX_HOME at a home of its own.
func CodexCommand(home string, args ...string) *exec.Cmd {
	cmd := exec.Command("codex", args...)
	cmd.Env = CodexEnv(home)
	return cmd
}

// CodexEnv is this process's environment with CODEX_HOME at ~/.codex under home, for anything else that writes Codex's configuration.
func CodexEnv(home string) []string {
	return append(os.Environ(), "CODEX_HOME="+filepath.Join(home, ".codex"))
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
	case "codex":
		if !codexModel.MatchString(a.Model) {
			return fmt.Errorf("codex model %q is not a model slug; run `codex debug models`", a.Model)
		}
		if !slices.Contains(codexEfforts, a.Effort) {
			return fmt.Errorf("codex has no effort %q; choose %s", a.Effort, strings.Join(codexEfforts[1:], ", "))
		}
		// Which efforts apply depends on the model the session picks, which nothing here can check.
		if a.Model == "inherit" && a.Effort != "" {
			return fmt.Errorf("codex inherit takes no effort; name a model to set one")
		}
	default:
		return fmt.Errorf("unknown client %q; choose claude, codex, or opencode", a.Client)
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
func StatePath(home string) string { return statefile.Path(home) }

// Load reads the models block, treating a missing file or block as no choice.
func Load(path string) (Choice, error) {
	doc, err := statefile.Read(path)
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
		if retired[role] {
			delete(c.Roles, role)
			continue
		}
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
	doc, err := statefile.Read(path)
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
	return statefile.Write(path, doc)
}
