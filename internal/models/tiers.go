package models

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// This file is tier handling: which client and model serve each kind of work,
// and the `harness models tier` commands that change it.

// Tiers name the kinds of work a model is meant for, strongest first.
var Tiers = []string{"heavy", "high", "standard", "fast"}

// fallback is the tier a tier falls back to when its model is unavailable.
var fallback = map[string]string{"heavy": "high"}

// tierModels is each client's default model family for a tier: Claude's
// alias, and the suffix of Codex's model slugs. OpenCode has no tier names.
// Claude's heavy defaults to opus, since fable is not on every plan; a human
// who has fable names it with tier set heavy --client claude --model fable.
var tierModels = map[string]map[string]string{
	"claude": {"heavy": "opus", "high": "opus", "standard": "sonnet", "fast": "haiku"},
	"codex":  {"heavy": "astra", "high": "sol", "standard": "terra", "fast": "luna"},
}

// claudeTiers is the tier each Claude alias was made for.
var claudeTiers = map[string]string{"fable": "heavy", "opus": "high", "sonnet": "standard", "haiku": "fast"}

const defaultPreset = "balanced"

// Presets choose the client that serves each tier.
var Presets = map[string]map[string]string{
	"balanced": {"heavy": "claude", "high": "claude", "standard": "codex", "fast": "claude"},
	"claude":   {"heavy": "claude", "high": "claude", "standard": "claude", "fast": "claude"},
	"codex":    {"heavy": "codex", "high": "codex", "standard": "codex", "fast": "codex"},
}

var retiredPresets = map[string]bool{"performance": true, "economy": true, "diversity": true}

// Binding is the client that serves a tier, and any model named for a
// client instead of the tier's newest.
type Binding struct {
	Client string            `json:"client,omitempty"`
	Models map[string]string `json:"models,omitempty"`
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
	return "", fmt.Errorf("opencode has no model for the %s tier; run `harness models tier set %s --client opencode --model <provider/model>`: %w", tier, tier, ErrNotListed)
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
	if client == "claude" {
		return claudeTiers[model]
	}
	for tier, family := range tierModels[client] {
		if strings.HasSuffix(model, "-"+family) {
			return tier
		}
	}
	return ""
}

// pin names the model a client runs for a tier, unless one is named already.
func (c *Choice) pin(tier, client, model string) {
	b := c.Tiers[tier]
	if b.Models[client] != "" {
		return
	}
	b.Models = maps.Clone(b.Models)
	if b.Models == nil {
		b.Models = map[string]string{}
	}
	b.Models[client] = model
	if c.Tiers == nil {
		c.Tiers = map[string]Binding{}
	}
	c.Tiers[tier] = b
}

func knownTier(tier string) error {
	if !slices.Contains(Tiers, tier) {
		return fmt.Errorf("unknown tier %q; choose %s", tier, strings.Join(Tiers, ", "))
	}
	return nil
}

func checkBinding(tier string, b Binding) error {
	if err := knownTier(tier); err != nil {
		return err
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

func presetNames() []string {
	names := make([]string, 0, len(Presets))
	for name := range Presets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// tierCommand runs `harness models tier <sub>` and reports whether it saved.
func tierCommand(path string, args []string, stdout io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, errors.New("tier takes show, preset, set, or reset")
	}
	sub, rest := args[0], args[1:]
	if sub == "reset" {
		return resetTier(path, rest)
	}
	choice, err := Load(path)
	if err != nil {
		return false, err
	}
	switch sub {
	case "show":
		if len(rest) > 0 {
			return false, errors.New("tier show takes no arguments")
		}
		showTiers(choice, new(Catalog), stdout)
		return false, nil
	case "preset":
		choice, err = setPreset(choice, rest)
	case "set":
		choice, err = setTier(choice, rest, new(Catalog))
	default:
		return false, fmt.Errorf("unknown tier command %q; choose show, preset, set, or reset", sub)
	}
	if err != nil {
		return false, err
	}
	return true, Save(path, choice)
}

func showTiers(choice Choice, cat *Catalog, stdout io.Writer) {
	for _, tier := range Tiers {
		b, source := choice.binding(tier)
		model, err := choice.Model(tier, b.Client, cat)
		note := problem(err)
		if next, ok := fallback[tier]; ok && errors.Is(err, ErrNotListed) {
			note = " [" + err.Error() + "; roles at " + tier + " fall back to " + next + "]"
		}
		fmt.Fprintf(stdout, "%s client=%s model=%s (%s)%s\n", tier, b.Client, orUnavailable(model), source, note)
	}
}

// setPreset applies `tier preset <name>`.
func setPreset(choice Choice, args []string) (Choice, error) {
	if len(args) != 1 {
		return choice, fmt.Errorf("tier preset takes one name: %s", strings.Join(presetNames(), ", "))
	}
	if _, ok := Presets[args[0]]; !ok {
		return choice, fmt.Errorf("unknown preset %q; choose %s", args[0], strings.Join(presetNames(), ", "))
	}
	choice.Preset = args[0]
	return choice, nil
}

// setTier applies `tier set <tier> --client <c> [--model <m>]`.
func setTier(choice Choice, args []string, cat *Catalog) (Choice, error) {
	const want = "tier set takes <tier> --client <c> [--model <m>]"
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return choice, errors.New(want)
	}
	tier := args[0]
	fs := flag.NewFlagSet("tier set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	client := fs.String("client", "", "")
	model := fs.String("model", "", "")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() > 0 || *client == "" {
		return choice, errors.New(want)
	}
	if err := knownTier(tier); err != nil {
		if knownRole(tier) == nil {
			return choice, fmt.Errorf("%s is a role; set it with `harness models role set %s`", tier, tier)
		}
		return choice, err
	}
	b := choice.Tiers[tier]
	b.Client = *client
	if *model != "" {
		if err := modelShape(*client, *model); err != nil {
			return choice, err
		}
		if err := Runnable(*client, *model, "", cat); err != nil {
			return choice, err
		}
		b.Models = maps.Clone(b.Models)
		if b.Models == nil {
			b.Models = map[string]string{}
		}
		b.Models[*client] = *model
	} else if b.Models[*client] != "" {
		b.Models = maps.Clone(b.Models)
		delete(b.Models, *client)
	}
	if err := checkBinding(tier, b); err != nil {
		return choice, err
	}
	if *client == "opencode" && b.Models["opencode"] == "" {
		return choice, fmt.Errorf("opencode has no tier names; serve %s with --model <provider/model>", tier)
	}
	choice.Tiers = maps.Clone(choice.Tiers)
	if choice.Tiers == nil {
		choice.Tiers = map[string]Binding{}
	}
	choice.Tiers[tier] = b
	return choice, nil
}

// resetTier drops one tier's setting, or every tier setting and the preset.
// It reads the stored block without validating it, so it can clear a bad one.
func resetTier(path string, args []string) (bool, error) {
	if len(args) > 1 {
		return false, errors.New("tier reset takes at most one tier")
	}
	choice, err := stored(path)
	if err != nil {
		return false, err
	}
	if len(args) == 0 {
		if choice.Preset == "" && len(choice.Tiers) == 0 {
			return false, nil
		}
		choice.Preset, choice.Tiers = "", nil
		return true, Save(path, choice)
	}
	if err := knownTier(args[0]); err != nil {
		return false, err
	}
	if _, ok := choice.Tiers[args[0]]; !ok {
		return false, nil
	}
	delete(choice.Tiers, args[0])
	return true, Save(path, choice)
}
