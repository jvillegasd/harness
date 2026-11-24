package models

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/agents"
	"github.com/jvillegasd/harness/internal/ui"
)

// This file is role handling: the tier, client, and effort each role runs at,
// and the `harness models role` commands that change and resolve it.

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

// Override is what the human changed about one role.
type Override struct {
	Tier   string `json:"tier,omitempty"`
	Client string `json:"client,omitempty"`
	Effort string `json:"effort,omitempty"`
	// Model is read only to migrate an override saved before tiers.
	Model string `json:"model,omitempty"`
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
		defaults.Tier, defaults.Effort = cmp.Or(reviewer.Tier, defaults.Tier), cmp.Or(reviewer.Effort, defaults.Effort)
		defaultClient = reviewer.Client
	}
	out := Resolved{Role: name, Tier: cmp.Or(launch.Tier, ov.Tier, defaults.Tier), Effort: cmp.Or(launch.Effort, ov.Effort, defaults.Effort)}
	source := ""
	switch {
	case launch != Launch{}:
		source = "launch"
	case overridden:
		source = "override"
	}
	if out.Tier == Session {
		out.Client, out.Model, out.Effort, out.Source = Session, "inherit", "", source
		if ov.Client != "" || ov.Effort != "" {
			out.Note = "the reviewer runs at the session tier, so this lens's client and effort settings are unused"
		}
		return out
	}
	for {
		b, tierSource := c.binding(out.Tier)
		out.Client = cmp.Or(launch.Client, ov.Client, defaultClient, b.Client)
		out.Source = cmp.Or(source, tierSource)
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
		// A role at the session tier is left out too, so every client inherits.
		if r.Err == nil && r.Tier != Session {
			out[name] = agents.Assigned{Client: r.Client, Model: r.Model, Effort: r.Effort, Source: r.Tier + ", " + r.Source}
		}
	}
	return out, all
}

// migrate turns an override saved before tiers into one: a Claude or Codex
// model into the tier it implies, pinned there when the tier's default is a
// different Claude model, and an OpenCode model into the model that serves
// the role's tier on OpenCode. It reports false for nothing to keep.
func (c *Choice) migrate(name string, ov *Override) bool {
	model := ov.Model
	ov.Model = ""
	if ov.Client != "opencode" {
		ov.Tier = cmp.Or(ov.Tier, TierOf(ov.Client, model))
		if ov.Tier == "" {
			return false
		}
		if ov.Client == "claude" && tierModels["claude"][ov.Tier] != model {
			c.pin(ov.Tier, "claude", model)
		}
		return true
	}
	if !strings.Contains(model, "/") {
		return false
	}
	tier := role(name).Tier
	if tier == "" {
		tier = role("reviewer").Tier
	}
	c.pin(tier, "opencode", model)
	return true
}

func knownRole(name string) error {
	if !slices.Contains(AllRoles, name) {
		return fmt.Errorf("unknown role %q; choose %s", name, strings.Join(AllRoles, ", "))
	}
	return nil
}

func checkOverride(ov Override) error {
	if ov.Tier != "" {
		if err := knownRoleTier(ov.Tier); err != nil {
			return err
		}
	}
	if ov.Tier == Session && (ov.Client != "" || ov.Effort != "") {
		return errors.New("a role at the session tier runs on the session's client and effort, so it takes no --client or --effort")
	}
	if ov.Client != "" && !slices.Contains(Clients, ov.Client) {
		return fmt.Errorf("unknown client %q; choose %s", ov.Client, strings.Join(Clients, ", "))
	}
	if !slices.Contains(slices.Concat(codexEfforts, openaiEfforts), ov.Effort) {
		return fmt.Errorf("unknown effort %q", ov.Effort)
	}
	return nil
}

// roleCommand runs `harness models role <sub>` and reports whether it saved.
func roleCommand(path string, args []string, stdout io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, errors.New("role takes show, set, reset, or resolve")
	}
	sub, rest := args[0], args[1:]
	if sub == "reset" {
		return resetRole(path, rest)
	}
	choice, err := Load(path)
	if err != nil {
		return false, err
	}
	switch sub {
	case "show":
		asJSON, err := jsonFlag("role show", rest)
		if err != nil {
			return false, err
		}
		rep := choice.report(new(Catalog))
		if asJSON {
			return false, writeJSON(stdout, rep.Roles)
		}
		drawRoles(ui.For(stdout), rep.Roles, stdout)
		return false, nil
	case "resolve":
		return false, resolve(choice, rest, stdout)
	case "set":
		if choice, err = setRole(choice, rest, new(Catalog)); err != nil {
			return false, err
		}
		return true, Save(path, choice)
	}
	return false, fmt.Errorf("unknown role command %q; choose show, set, reset, or resolve", sub)
}

// setRole applies `role set <role> [--tier <t>] [--client <c>] [--effort <e>]`,
// merging into the role's override so what it leaves out stays.
func setRole(choice Choice, args []string, cat *Catalog) (Choice, error) {
	const want = "role set takes <role> and at least one of --tier <t>, --client <c>, --effort <e>"
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return choice, errors.New(want)
	}
	name := args[0]
	fs := flag.NewFlagSet("role set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var given Override
	fs.StringVar(&given.Tier, "tier", "", "")
	fs.StringVar(&given.Client, "client", "", "")
	fs.StringVar(&given.Effort, "effort", "", "")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() > 0 || given == (Override{}) {
		return choice, errors.New(want)
	}
	if err := knownRole(name); err != nil {
		if knownTier(name) == nil {
			return choice, fmt.Errorf("%s is a tier; set it with `harness models tier set %s --client <c>`", name, name)
		}
		return choice, err
	}
	ov := choice.Roles[name]
	if given.Tier == Session {
		// The session tier replaces the role's client and effort, so it drops them.
		ov = Override{}
	}
	ov.Tier, ov.Client, ov.Effort = cmp.Or(given.Tier, ov.Tier), cmp.Or(given.Client, ov.Client), cmp.Or(given.Effort, ov.Effort)
	if err := checkOverride(ov); err != nil {
		return choice, err
	}
	choice.Roles = maps.Clone(choice.Roles)
	if choice.Roles == nil {
		choice.Roles = map[string]Override{}
	}
	choice.Roles[name] = ov
	if r := choice.Resolve(name, Launch{}, cat); r.EffortErr != nil && ov.Effort != "" {
		return choice, r.EffortErr
	} else if r.Err != nil && ov.Client == "opencode" {
		return choice, r.Err
	}
	return choice, nil
}

// resetRole drops one role's override, or every override, starting from
// forReset so it clears a migrated override and a bad one alike.
func resetRole(path string, args []string) (bool, error) {
	if len(args) > 1 {
		return false, errors.New("role reset takes at most one role")
	}
	choice, err := forReset(path)
	if err != nil {
		return false, err
	}
	if len(args) == 0 {
		if len(choice.Roles) == 0 {
			return false, nil
		}
		choice.Roles = nil
		return true, Save(path, choice)
	}
	if err := knownRole(args[0]); err != nil {
		return false, err
	}
	if _, ok := choice.Roles[args[0]]; !ok {
		return false, nil
	}
	delete(choice.Roles, args[0])
	return true, Save(path, choice)
}

func resolve(choice Choice, args []string, stdout io.Writer) error {
	const want = "role resolve takes <role> [--tier <t>] [--client <c>] [--effort <e>] [--json]"
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New(want)
	}
	name := args[0]
	fs := flag.NewFlagSet("role resolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "")
	var launch Launch
	fs.StringVar(&launch.Tier, "tier", "", "")
	fs.StringVar(&launch.Client, "client", "", "")
	fs.StringVar(&launch.Effort, "effort", "", "")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() > 0 {
		return errors.New(want)
	}
	if err := knownRole(name); err != nil {
		return err
	}
	if launch != (Launch{}) {
		if !role(name).Generic {
			return fmt.Errorf("%s runs at its configured tier; only a generic role (worker) takes --tier, --client, or --effort at launch", name)
		}
		if err := checkOverride(Override{Tier: launch.Tier, Client: launch.Client, Effort: launch.Effort}); err != nil {
			return err
		}
	}
	r := choice.Resolve(name, launch, new(Catalog))
	if r.EffortErr != nil && launch.Effort != "" {
		return r.EffortErr
	}
	if r.Err != nil {
		return fmt.Errorf("%s has no runnable model here: %v", name, r.Err)
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(view(r))
	}
	drawRoles(ui.For(stdout), []roleView{view(r)}, stdout)
	return nil
}
