package models

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
)

const usage = `usage: harness models [show|set|reset|resolve]

  (no arguments, in a terminal)                 Pick a preset, or each role's client, model, and effort
  show                                          Print each role's client, model, effort, and source
  set --preset <name>                           Choose a preset: balanced (default), performance, economy, diversity
  set <role> --client <c> --model <m> [--effort <e>]   Override one role
  reset [<role>]                                Drop one override, or the whole choice
  resolve <role> --session <client> [--json]    Print the role's assignment and how that session launches it

The choice is stored in ~/.harness/state.json; ` + "`harness install`" + ` renders it into each client's role agents.
`

// Command runs `harness models`.
func Command(args []string, stdout, stderr io.Writer) int {
	// Without a terminal the picker would wait on input that never comes, so a script gets the usage instead.
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

// pickAndSave runs the picker, saves what it collected, and reports the result through show.
func pickAndSave(path string, stdout io.Writer) error {
	if _, err := Load(path); err != nil {
		return err
	}
	final, err := tea.NewProgram(newPicker(OpenCodeModels()), tea.WithOutput(stdout), tea.WithInput(os.Stdin)).Run()
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
	// reset is the recovery for a stored choice Load refuses, so it reads the block without validating it.
	if sub == "reset" {
		return reset(path, rest)
	}
	choice, err := Load(path)
	if err != nil {
		return err
	}
	switch sub {
	case "show":
		resolved := choice.Resolve()
		var listed []string
		var listErr error
		for _, role := range Roles {
			r := resolved[role]
			note := ""
			if r.Client == "opencode" {
				if listed == nil && listErr == nil {
					listed, listErr = OpenCodeModels()
				}
				switch {
				case listErr != nil:
					note = " [unverified: " + listErr.Error() + "]"
				case !slices.Contains(listed, r.Model):
					note = " [not listed by `opencode models`; install renders the client default]"
				}
			}
			fmt.Fprintf(stdout, "%s client=%s model=%s effort=%s (%s)%s\n", role, r.Client, r.Model, effortOrInherit(r.Effort), r.Source, note)
		}
		return nil
	case "set":
		choice, err := apply(choice, rest)
		if err != nil {
			return err
		}
		return Save(path, choice)
	case "resolve":
		return resolve(choice, rest, stdout)
	}
	return fmt.Errorf("unknown subcommand %q\n%s", sub, usage)
}

// apply validates one set invocation's arguments and returns the choice with it applied. The picker saves through it too, so both paths accept exactly the same values.
func apply(choice Choice, args []string) (Choice, error) {
	var role string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		role, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	preset := fs.String("preset", "", "")
	a := Assignment{}
	fs.StringVar(&a.Client, "client", "", "")
	fs.StringVar(&a.Model, "model", "", "")
	fs.StringVar(&a.Effort, "effort", "", "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return choice, fmt.Errorf("set takes --preset <name>, or <role> --client --model [--effort]")
	}
	if role == "" {
		if _, ok := Presets[*preset]; !ok {
			return choice, fmt.Errorf("unknown preset %q; choose %s", *preset, strings.Join(presetNames(), ", "))
		}
		// A built-in preset is well formed by construction; whether this machine can run its OpenCode models is the install's fallback to judge.
		choice.Preset = *preset
		return choice, nil
	}
	if *preset != "" {
		return choice, fmt.Errorf("set a preset or a role, not both")
	}
	if err := knownRole(role); err != nil {
		return choice, err
	}
	if err := Validate(a); err != nil {
		return choice, err
	}
	if choice.Roles == nil {
		choice.Roles = map[string]Assignment{}
	}
	choice.Roles[role] = a
	return choice, nil
}

func reset(path string, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("reset takes at most one role")
	}
	if len(args) == 0 {
		return Save(path, Choice{})
	}
	if err := knownRole(args[0]); err != nil {
		return err
	}
	doc, err := readDoc(path)
	if err != nil {
		return err
	}
	var choice Choice
	if raw, ok := doc["models"]; ok {
		if err := json.Unmarshal(raw, &choice); err != nil {
			return fmt.Errorf("%s: models: %w; run `harness models reset` with no role", path, err)
		}
	}
	if _, ok := choice.Roles[args[0]]; !ok {
		return nil
	}
	delete(choice.Roles, args[0])
	return Save(path, choice)
}

func resolve(choice Choice, args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("resolve takes <role> --session <client> [--json]")
	}
	role := args[0]
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	session := fs.String("session", "", "")
	asJSON := fs.Bool("json", false, "")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() > 0 {
		return fmt.Errorf("resolve takes <role> --session <client> [--json]")
	}
	if err := knownRole(role); err != nil {
		return err
	}
	if *session != "claude" && *session != "opencode" {
		return fmt.Errorf("resolve needs --session claude or --session opencode")
	}
	r := choice.Resolve()[role]
	out := map[string]string{"role": role, "client": r.Client, "model": r.Model, "effort": effortOrInherit(r.Effort), "source": r.Source, "route": Route(r.Assignment, *session, "")}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(out)
	}
	_, err := fmt.Fprintf(stdout, "%s client=%s model=%s effort=%s (%s) route=%s\n", role, out["client"], out["model"], out["effort"], out["source"], out["route"])
	return err
}

func knownRole(role string) error {
	if !slices.Contains(Roles, role) {
		return fmt.Errorf("unknown role %q; choose %s", role, strings.Join(Roles, ", "))
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
