package models

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/jvillegasd/harness/internal/statefile"
)

const usage = `usage: harness models [show|set|reset|resolve]

  (no arguments, in a terminal)                       Pick a preset, or set tiers and roles one by one
  show                                                Print each tier's model, then each role's tier, model, and effort
  set --preset <name>                                 Choose which client serves each tier: balanced (default), claude, codex
  set <tier> --client <c> [--model <m>]               Serve a tier from a client, on a named model or the tier's newest
  set <role> [--tier <t>] [--client <c>] [--effort <e>]   Override one role
  reset [<tier>|<role>]                               Drop one tier or role setting, or the whole choice
  resolve <role> [--tier <t>] [--client <c>] [--effort <e>] [--json]
                                                      Print the role's tier, client, model, effort, and source;
                                                      --tier, --client, and --effort are the worker's launch choices

Tiers, strongest first: heavy (system design, architecture), high (defects, engineering, analysis),
standard (scoped work with tests), fast (classification, gathering data in volume, boilerplate).
Claude runs fable, opus, sonnet, haiku; Codex its newest listed -astra, -sol, -terra, -luna model;
OpenCode only a model named with set <tier>. heavy falls back to high when no heavy model is listed.

The choice is stored in ~/.harness/state.json; ` + "`harness install`" + ` renders it into each client's role agents.
`

func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "read-guard" {
		return ReadGuard(os.Stdin, stdout)
	}
	if len(args) == 1 && args[0] == "secret-guard" {
		return SecretGuard(os.Stdin, stdout)
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(stdout, usage)
		return 0
	}

	if len(args) == 0 && (!term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd())) {
		fmt.Fprint(stderr, usage)
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "harness models: %v\n", err)
		return 1
	}
	path := StatePath(home)
	if len(args) == 0 {
		if err := pickAndSave(path, stdout); err != nil {
			fmt.Fprintf(stderr, "harness models: %v\n", err)
			return 1
		}
		return 0
	}
	if err := run(path, args, stdout); err != nil {
		fmt.Fprintf(stderr, "harness models: %v\n", err)
		return 1
	}
	return 0
}

func pickAndSave(path string, stdout io.Writer) error {
	choice, err := Load(path)
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(newPicker(new(Catalog), choice), tea.WithOutput(stdout), tea.WithInput(os.Stdin)).Run()
	if err != nil {
		return err
	}
	wrote, err := final.(picker).save(path)
	if err != nil {
		return err
	}
	if !wrote {
		_, err := fmt.Fprintln(stdout, "nothing written")
		return err
	}
	return run(path, []string{"show"}, stdout)
}

func run(path string, args []string, stdout io.Writer) error {
	sub, rest := args[0], args[1:]

	if sub == "reset" {
		return reset(path, rest)
	}
	choice, err := Load(path)
	if err != nil {
		return err
	}
	switch sub {
	case "show":
		cat := new(Catalog)
		for _, tier := range Tiers {
			b, source := choice.binding(tier)
			model, err := choice.Model(tier, b.Client, cat)
			note := problem(err)
			if next, ok := fallback[tier]; ok && errors.Is(err, ErrNotListed) {
				note = " [" + err.Error() + "; roles at " + tier + " fall back to " + next + "]"
			}
			fmt.Fprintf(stdout, "%s client=%s model=%s (%s)%s\n", tier, b.Client, orUnavailable(model), source, note)
		}
		for _, name := range AllRoles {
			fmt.Fprintln(stdout, line(choice.Resolve(name, Launch{}, cat)))
		}
		return nil
	case "set":
		choice, err := apply(choice, rest, new(Catalog))
		if err != nil {
			return err
		}
		return Save(path, choice)
	case "resolve":
		return resolve(choice, rest, stdout)
	}
	return fmt.Errorf("unknown subcommand %q\n%s", sub, usage)
}

func line(r Resolved) string {
	text := fmt.Sprintf("%s tier=%s client=%s model=%s effort=%s (%s)", r.Role, r.Tier, r.Client, orUnavailable(r.Model), effortOrInherit(r.Effort), r.Source)
	if r.Note != "" {
		text += " [" + r.Note + "]"
	}
	return text + problem(r.Err)
}

func orUnavailable(model string) string {
	if model == "" {
		return "unavailable"
	}
	return model
}

func problem(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNotListed):
		return " [" + err.Error() + "; install renders the client default]"
	}
	return " [unverified: " + err.Error() + "]"
}

func apply(choice Choice, args []string, cat *Catalog) (Choice, error) {
	var name string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	preset := fs.String("preset", "", "")
	tier := fs.String("tier", "", "")
	client := fs.String("client", "", "")
	model := fs.String("model", "", "")
	effort := fs.String("effort", "", "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return choice, fmt.Errorf("set takes --preset <name>, <tier> --client [--model], or <role> [--tier] [--client] [--effort]")
	}
	if name == "" {
		if _, ok := Presets[*preset]; !ok {
			return choice, fmt.Errorf("unknown preset %q; choose %s", *preset, strings.Join(presetNames(), ", "))
		}
		choice.Preset = *preset
		return choice, nil
	}
	if *preset != "" {
		return choice, fmt.Errorf("set a preset, a tier, or a role, not two")
	}
	if slices.Contains(Tiers, name) {
		return setTier(choice, name, *client, *model, *tier+*effort, cat)
	}
	if err := knownRole(name); err != nil {
		return choice, fmt.Errorf("%v, or a tier: %s", err, strings.Join(Tiers, ", "))
	}
	if *model != "" {
		return choice, fmt.Errorf("a role takes a tier, not a model; name the model with set <tier> --client <c> --model <m>")
	}
	ov := Override{Tier: *tier, Client: *client, Effort: *effort}
	if ov == (Override{}) {
		return choice, fmt.Errorf("set %s takes --tier, --client, or --effort", name)
	}
	if err := checkOverride(name, ov); err != nil {
		return choice, err
	}
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

func setTier(choice Choice, tier, client, model, extra string, cat *Catalog) (Choice, error) {
	if extra != "" {
		return choice, fmt.Errorf("set %s takes --client and --model only", tier)
	}
	if client == "" {
		return choice, fmt.Errorf("set %s takes --client <c> [--model <m>]", tier)
	}
	b := choice.Tiers[tier]
	b.Client = client
	if model != "" {
		if err := modelShape(client, model); err != nil {
			return choice, err
		}
		if err := Runnable(client, model, "", cat); err != nil {
			return choice, err
		}
		b.Models = maps.Clone(b.Models)
		if b.Models == nil {
			b.Models = map[string]string{}
		}
		b.Models[client] = model
	} else if b.Models[client] != "" {
		b.Models = maps.Clone(b.Models)
		delete(b.Models, client)
	}
	if err := checkBinding(tier, b); err != nil {
		return choice, err
	}
	if client == "opencode" && model == "" && b.Models["opencode"] == "" {
		return choice, fmt.Errorf("opencode has no tier names; serve %s with --model <provider/model>", tier)
	}
	choice.Tiers = maps.Clone(choice.Tiers)
	if choice.Tiers == nil {
		choice.Tiers = map[string]Binding{}
	}
	choice.Tiers[tier] = b
	return choice, nil
}

func reset(path string, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("reset takes at most one tier or role")
	}
	if len(args) == 0 {
		return Save(path, Choice{})
	}
	name := args[0]
	if !slices.Contains(Tiers, name) {
		if err := knownRole(name); err != nil {
			return err
		}
	}
	doc, err := statefile.Read(path)
	if err != nil {
		return err
	}
	var choice Choice
	if raw, ok := doc["models"]; ok {
		if err := json.Unmarshal(raw, &choice); err != nil {
			return fmt.Errorf("%s: models: %w; run `harness models reset` with no name", path, err)
		}
	}
	_, isTier := choice.Tiers[name]
	_, isRole := choice.Roles[name]
	if !isTier && !isRole {
		return nil
	}
	delete(choice.Tiers, name)
	delete(choice.Roles, name)
	return Save(path, choice)
}

func resolve(choice Choice, args []string, stdout io.Writer) error {
	const want = "resolve takes <role> [--tier <t>] [--client <c>] [--effort <e>] [--json]"
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New(want)
	}
	name := args[0]
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
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
		if err := checkOverride(name, Override{Tier: launch.Tier, Client: launch.Client, Effort: launch.Effort}); err != nil {
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
		return json.NewEncoder(stdout).Encode(map[string]string{"role": name, "tier": r.Tier, "client": r.Client, "model": r.Model, "effort": effortOrInherit(r.Effort), "source": r.Source, "note": r.Note})
	}
	_, err := fmt.Fprintln(stdout, line(r))
	return err
}

func knownRole(role string) error {
	if !slices.Contains(AllRoles, role) {
		return fmt.Errorf("unknown role %q; choose %s", role, strings.Join(AllRoles, ", "))
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

func effortOrInherit(e string) string {
	if e == "" {
		return "inherit"
	}
	return e
}
