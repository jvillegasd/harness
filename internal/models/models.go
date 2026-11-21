package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jvillegasd/harness/internal/agents"
	"github.com/jvillegasd/harness/internal/statefile"
)

// Tiers name the kinds of work a model is meant for, strongest first.
var Tiers = []string{"heavy", "high", "standard", "fast"}

// fallback is the tier a tier falls back to when its model is unavailable.
var fallback = map[string]string{"heavy": "high"}

// tierModels is each client's model family for a tier: Claude's alias, and
// the suffix of Codex's model slugs. OpenCode has no tier names.
var tierModels = map[string]map[string]string{
	"claude": {"heavy": "fable", "high": "opus", "standard": "sonnet", "fast": "haiku"},
	"codex":  {"heavy": "astra", "high": "sol", "standard": "terra", "fast": "luna"},
}

var Clients = []string{"claude", "codex", "opencode"}

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

func role(name string) agents.Role {
	for _, r := range agents.Roles {
		if r.Name == name {
			return r
		}
	}
	return agents.Role{}
}

var retired = map[string]bool{"reviewer-a": true, "reviewer-b": true, "review-refuter": true, "review-standards": true}

var retiredPresets = map[string]bool{"performance": true, "economy": true, "diversity": true}

const defaultPreset = "balanced"

// Presets choose the client that serves each tier.
var Presets = map[string]map[string]string{
	"balanced": {"heavy": "claude", "high": "claude", "standard": "codex", "fast": "claude"},
	"claude":   {"heavy": "claude", "high": "claude", "standard": "claude", "fast": "claude"},
	"codex":    {"heavy": "codex", "high": "codex", "standard": "codex", "fast": "codex"},
}

var claudeEfforts = map[string][]string{
	"fable":  {"", "low", "medium", "high", "xhigh", "max"},
	"opus":   {"", "low", "medium", "high", "xhigh", "max"},
	"sonnet": {"", "low", "medium", "high", "xhigh", "max"},
	"haiku":  {""},
}

var openaiEfforts = []string{"", "none", "minimal", "low", "medium", "high", "xhigh"}

var codexEfforts = []string{"", "low", "medium", "high", "xhigh", "max", "ultra"}

var (
	claudeModel = regexp.MustCompile(`^(fable|opus|sonnet|haiku|claude-[a-z0-9.-]+)$`)
	codexModel  = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
)

// Binding is the client that serves a tier, and any model named for a
// client instead of the tier's newest.
type Binding struct {
	Client string            `json:"client,omitempty"`
	Models map[string]string `json:"models,omitempty"`
}

// Override is what the human changed about one role.
type Override struct {
	Tier   string `json:"tier,omitempty"`
	Client string `json:"client,omitempty"`
	Effort string `json:"effort,omitempty"`
	// Model is read only to migrate an override saved before tiers.
	Model string `json:"model,omitempty"`
}

type Choice struct {
	Preset string              `json:"preset,omitempty"`
	Tiers  map[string]Binding  `json:"tiers,omitempty"`
	Roles  map[string]Override `json:"roles,omitempty"`
}

var ErrNotListed = errors.New("not listed")

type Catalog struct {
	opencode, codex         []string
	codexEfforts            map[string][]string
	opencodeErr, codexErr   error
	opencodeDone, codexDone bool
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
	if _, err := c.Codex(); err != nil {
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

// newestCodex returns the listed slug of the family with the highest version,
// such as gpt-6.2-sol over gpt-6.1-sol.
func newestCodex(listed []string, family string) string {
	best, bestVersion := "", []int(nil)
	for _, slug := range listed {
		rest, ok := strings.CutSuffix(slug, "-"+family)
		if !ok {
			continue
		}
		version := versionOf(rest)
		if best == "" || slices.Compare(version, bestVersion) > 0 {
			best, bestVersion = slug, version
		}
	}
	return best
}

func versionOf(s string) []int {
	var out []int
	for _, part := range regexp.MustCompile(`[0-9]+`).FindAllString(s, -1) {
		n, _ := strconv.Atoi(part)
		out = append(out, n)
	}
	return out
}

// TierOf names the tier of a Claude alias or a Codex slug, or "".
func TierOf(client, model string) string {
	for tier, family := range tierModels[client] {
		if model == family || (client == "codex" && strings.HasSuffix(model, "-"+family)) {
			return tier
		}
	}
	return ""
}

// binding is the tier's binding with the preset's client filled in.
func (c Choice) binding(tier string) (Binding, string) {
	preset := c.preset()
	b, set := c.Tiers[tier]
	source := "preset:" + preset
	if set {
		source = "set"
	}
	if b.Client == "" {
		b.Client = Presets[preset][tier]
	}
	return b, source
}

func (c Choice) preset() string {
	if c.Preset == "" {
		return defaultPreset
	}
	return c.Preset
}

// Model resolves the model a client runs for a tier: the one named for the
// tier, else the tier's alias on Claude or newest listed slug on Codex.
// OpenCode has no tier names, so it needs a named model.
func (c Choice) Model(tier, client string, cat *Catalog) (string, error) {
	b, _ := c.binding(tier)
	if model := b.Models[client]; model != "" {
		return model, Runnable(client, model, "", cat)
	}
	switch client {
	case "claude":
		return tierModels["claude"][tier], nil
	case "codex":
		listed, err := cat.Codex()
		if err != nil {
			return "", err
		}
		if model := newestCodex(listed, tierModels["codex"][tier]); model != "" {
			return model, nil
		}
		return "", fmt.Errorf("codex lists no %s (%s) model; run `codex debug models`: %w", tier, tierModels["codex"][tier], ErrNotListed)
	}
	return "", fmt.Errorf("opencode has no model for the %s tier; run `harness models set %s --client opencode --model <provider/model>`: %w", tier, tier, ErrNotListed)
}

// Runnable reports whether this machine lists the model, and the model the
// effort, when the client publishes a catalog.
func Runnable(client, model, effort string, cat *Catalog) error {
	switch client {
	case "claude":
		if efforts, ok := claudeEfforts[model]; ok && !slices.Contains(efforts, effort) {
			return fmt.Errorf("claude %s does not support effort %q", model, effort)
		}
	case "opencode":
		listed, err := cat.OpenCode()
		if err != nil {
			return err
		}
		if !slices.Contains(listed, model) {
			return fmt.Errorf("opencode has no model %q; run `opencode models`: %w", model, ErrNotListed)
		}
		if effort != "" && (!strings.HasPrefix(model, "openai/") || !slices.Contains(openaiEfforts, effort)) {
			return fmt.Errorf("opencode sends effort only as OpenAI's reasoningEffort, so %q is unsupported for %s", effort, model)
		}
	case "codex":
		listed, err := cat.Codex()
		if err != nil {
			return err
		}
		if !slices.Contains(listed, model) {
			return fmt.Errorf("codex has no model %q; run `codex debug models`: %w", model, ErrNotListed)
		}
		if effort != "" && !slices.Contains(cat.codexEfforts[model], effort) {
			return fmt.Errorf("codex %s does not support effort %q", model, effort)
		}
	}
	return nil
}

// Launch is what a parent chooses when it launches a generic role.
type Launch struct{ Tier, Client, Effort string }

type Resolved struct {
	Role, Tier, Client, Model, Effort string
	Source                            string
	// Note says what resolution changed: a tier that fell back, or an
	// effort the model does not take. Err says why the role has no model.
	Note      string
	EffortErr error
	Err       error
}

// Resolve resolves one role, with the launch choices a generic role takes.
// A lens without an override follows the reviewer.
func (c Choice) Resolve(name string, launch Launch, cat *Catalog) Resolved {
	r := role(name)
	ov, overridden := c.Roles[name]
	if r.Lens && !overridden {
		out := c.Resolve("reviewer", Launch{}, cat)
		out.Role, out.Source = name, "reviewer"
		return out
	}
	// An overridden lens takes what it did not set from the reviewer.
	defaults, defaultClient := r, ""
	if r.Lens {
		reviewer := c.Roles["reviewer"]
		defaults = role("reviewer")
		defaults.Tier, defaults.Effort = first(reviewer.Tier, defaults.Tier), first(reviewer.Effort, defaults.Effort)
		defaultClient = reviewer.Client
	}
	out := Resolved{Role: name, Tier: first(launch.Tier, ov.Tier, defaults.Tier), Effort: first(launch.Effort, ov.Effort, defaults.Effort)}
	source := ""
	switch {
	case launch != Launch{}:
		source = "launch"
	case overridden:
		source = "override"
	}
	for {
		b, tierSource := c.binding(out.Tier)
		out.Client = first(launch.Client, ov.Client, defaultClient, b.Client)
		out.Source = first(source, tierSource)
		out.Model, out.Err = c.Model(out.Tier, out.Client, cat)
		next, ok := fallback[out.Tier]
		if !errors.Is(out.Err, ErrNotListed) || !ok {
			break
		}
		out.Note = fmt.Sprintf("%s unavailable on %s, fell back to %s", out.Tier, out.Client, next)
		out.Tier = next
	}
	if out.Err == nil && out.Effort != "" {
		if err := Runnable(out.Client, out.Model, out.Effort, cat); err != nil && Runnable(out.Client, out.Model, "", cat) == nil {
			out.EffortErr = err
			out.Note = strings.TrimPrefix(out.Note+"; ", "; ") + err.Error() + ", so it runs at the client default effort"
			out.Effort = ""
		}
	}
	return out
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ResolveAll resolves every role at its configured tier.
func (c Choice) ResolveAll(cat *Catalog) map[string]Resolved {
	out := map[string]Resolved{}
	for _, name := range AllRoles {
		out[name] = c.Resolve(name, Launch{}, cat)
	}
	return out
}

// ForRender is every role's resolution in the form the agent renderer takes,
// leaving out a role with no runnable model so it keeps the client default.
func (c Choice) ForRender(cat *Catalog) (map[string]agents.Assigned, map[string]Resolved) {
	all := c.ResolveAll(cat)
	out := map[string]agents.Assigned{}
	for name, r := range all {
		if r.Err == nil {
			out[name] = agents.Assigned{Client: r.Client, Model: r.Model, Effort: r.Effort, Source: r.Tier + ", " + r.Source}
		}
	}
	return out, all
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
	if retiredPresets[c.Preset] {
		c.Preset = ""
	}
	if _, ok := Presets[c.Preset]; c.Preset != "" && !ok {
		return Choice{}, fmt.Errorf("%s: unknown preset %q; run `harness models reset`", path, c.Preset)
	}
	for tier, b := range c.Tiers {
		if err := checkBinding(tier, b); err != nil {
			return Choice{}, fmt.Errorf("%s: %v; run `harness models reset %s`", path, err, tier)
		}
	}
	for name, ov := range c.Roles {
		if retired[name] {
			delete(c.Roles, name)
			continue
		}
		if err := knownRole(name); err != nil {
			return Choice{}, fmt.Errorf("%s: %v; run `harness models reset`", path, err)
		}
		if ov.Model != "" {
			if !c.migrate(name, &ov) {
				delete(c.Roles, name)
				continue
			}
			c.Roles[name] = ov
		}
		if err := checkOverride(name, ov); err != nil {
			return Choice{}, fmt.Errorf("%s: %s: %v; run `harness models reset %s`", path, name, err, name)
		}
	}
	return c, nil
}

// migrate turns an override saved before tiers into one: a Claude or Codex
// model into the tier it implies, and an OpenCode model into the model that
// serves the role's tier on OpenCode. It reports false for nothing to keep.
func (c *Choice) migrate(name string, ov *Override) bool {
	model := ov.Model
	ov.Model = ""
	if ov.Client != "opencode" {
		ov.Tier = first(ov.Tier, TierOf(ov.Client, model))
		return ov.Tier != ""
	}
	if !strings.Contains(model, "/") {
		return false
	}
	tier := role(name).Tier
	if tier == "" {
		tier = role("reviewer").Tier
	}
	b := c.Tiers[tier]
	if b.Models["opencode"] == "" {
		b.Models = maps.Clone(b.Models)
		if b.Models == nil {
			b.Models = map[string]string{}
		}
		b.Models["opencode"] = model
		if c.Tiers == nil {
			c.Tiers = map[string]Binding{}
		}
		c.Tiers[tier] = b
	}
	return true
}

func checkBinding(tier string, b Binding) error {
	if !slices.Contains(Tiers, tier) {
		return fmt.Errorf("unknown tier %q; choose %s", tier, strings.Join(Tiers, ", "))
	}
	if b.Client != "" && !slices.Contains(Clients, b.Client) {
		return fmt.Errorf("unknown client %q; choose %s", b.Client, strings.Join(Clients, ", "))
	}
	for client, model := range b.Models {
		if err := modelShape(client, model); err != nil {
			return fmt.Errorf("%s: %v", tier, err)
		}
	}
	return nil
}

func modelShape(client, model string) error {
	switch client {
	case "claude":
		if !claudeModel.MatchString(model) {
			return fmt.Errorf("unknown Claude model %q; choose fable, opus, sonnet, haiku, or a claude- model id", model)
		}
	case "codex":
		if !codexModel.MatchString(model) {
			return fmt.Errorf("codex model %q is not a model slug; run `codex debug models`", model)
		}
	case "opencode":
		if !strings.Contains(model, "/") {
			return fmt.Errorf("opencode model %q is not a provider/model id", model)
		}
	default:
		return fmt.Errorf("unknown client %q; choose %s", client, strings.Join(Clients, ", "))
	}
	return nil
}

func checkOverride(name string, ov Override) error {
	if ov.Tier != "" && !slices.Contains(Tiers, ov.Tier) {
		return fmt.Errorf("unknown tier %q; choose %s", ov.Tier, strings.Join(Tiers, ", "))
	}
	if ov.Client != "" && !slices.Contains(Clients, ov.Client) {
		return fmt.Errorf("unknown client %q; choose %s", ov.Client, strings.Join(Clients, ", "))
	}
	all := slices.Concat(codexEfforts, openaiEfforts)
	if !slices.Contains(all, ov.Effort) {
		return fmt.Errorf("unknown effort %q", ov.Effort)
	}
	return nil
}

func Save(path string, c Choice) error {
	doc, err := statefile.Read(path)
	if err != nil {
		return err
	}
	if c.Preset == "" && len(c.Tiers) == 0 && len(c.Roles) == 0 {
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
