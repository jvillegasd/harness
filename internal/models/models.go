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

type Assignment struct {
	Client string `json:"client"`
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
}

var Roles, Lenses = func() (roles, lenses []string) {
	for _, r := range agents.Roles {
		if r.Lens {
			lenses = append(lenses, r.Name)
		} else {
			roles = append(roles, r.Name)
		}
	}
	return roles, lenses
}()

var AllRoles = append(append([]string{}, Roles...), Lenses...)

var retired = map[string]bool{"reviewer-a": true, "reviewer-b": true, "review-refuter": true, "review-standards": true}

const defaultPreset = "balanced"

func claude(model, effort string) Assignment { return Assignment{"claude", model, effort} }
func codex(model, effort string) Assignment  { return Assignment{"codex", model, effort} }

var Presets = map[string]map[string]Assignment{
	"balanced": {
		"executor": codex("gpt-6-luna", ""), "code-search": claude("haiku", ""), "reviewer": claude("sonnet", "medium"),
	},
	"performance": {
		"executor": claude("opus", "medium"), "code-search": claude("sonnet", ""), "reviewer": claude("fable", "medium"),
	},
	"economy": {
		"executor": codex("gpt-5.6-luna", ""), "code-search": claude("haiku", ""), "reviewer": claude("sonnet", "low"),
	},
	"diversity": {
		"executor": claude("sonnet", "medium"), "code-search": claude("haiku", ""), "reviewer": codex("gpt-6.1-sol", "low"),
	},
}

var claudeEfforts = map[string][]string{
	"inherit": {""},
	"fable":   {"", "low", "medium", "high", "xhigh", "max"},
	"opus":    {"", "low", "medium", "high", "xhigh", "max"},
	"sonnet":  {"", "low", "medium", "high", "xhigh", "max"},
	"haiku":   {""},
}

var openaiEfforts = []string{"", "none", "minimal", "low", "medium", "high", "xhigh"}

var codexModel = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

var codexEfforts = []string{"", "low", "medium", "high", "xhigh", "max", "ultra"}

func Validate(a Assignment) error {
	if err := shape(a); err != nil {
		return err
	}
	return new(Catalog).Runnable(a)
}

var ErrNotListed = errors.New("not listed")

type Catalog struct {
	opencode, codex         []string
	codexEfforts            map[string][]string
	opencodeErr, codexErr   error
	opencodeDone, codexDone bool
}

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

func (c *Catalog) Codex() ([]string, error) {
	if !c.codexDone {
		c.codexDone = true
		c.codex, c.codexEfforts, c.codexErr = codexCatalog()
	}
	return c.codex, c.codexErr
}

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

func CodexCommand(home string, args ...string) *exec.Cmd {
	cmd := exec.Command("codex", args...)
	cmd.Env = CodexEnv(home)
	return cmd
}

func CodexEnv(home string) []string {
	return append(os.Environ(), "CODEX_HOME="+filepath.Join(home, ".codex"))
}

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

		if a.Model == "inherit" && a.Effort != "" {
			return fmt.Errorf("codex inherit takes no effort; name a model to set one")
		}
	default:
		return fmt.Errorf("unknown client %q; choose claude, codex, or opencode", a.Client)
	}
	return nil
}

type Choice struct {
	Preset string                `json:"preset,omitempty"`
	Roles  map[string]Assignment `json:"roles,omitempty"`
}

type Resolved struct {
	Assignment
	Source string
}

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
	for _, lens := range Lenses {
		if a, ok := c.Roles[lens]; ok {
			out[lens] = Resolved{a, "override"}
			continue
		}
		out[lens] = Resolved{out["reviewer"].Assignment, "reviewer"}
	}
	return out
}

func (c Choice) ForRender() map[string]agents.Assigned {
	out := map[string]agents.Assigned{}
	for role, r := range c.Resolve() {
		out[role] = agents.Assigned{Client: r.Client, Model: r.Model, Effort: r.Effort, Source: r.Source}
	}
	return out
}

func StatePath(home string) string { return statefile.Path(home) }

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
