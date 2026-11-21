package models

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
)

const usage = `usage: harness models [show|reset|tier <command>|role <command>]

A tier is a kind of work, served by one client and model. A role runs at a tier,
and may run on another client or at another effort.

  (no arguments, in a terminal)                 Pick a preset, or set tiers and roles one by one
  show                                          Print every tier, then every role
  reset                                         Drop every tier and role setting

Tiers: which client and model serve each kind of work
  tier show                                     Print each tier's client and model
  tier preset <name>                            Choose which client serves each tier: balanced (default), claude, codex
  tier set <tier> --client <c> [--model <m>]    Serve a tier from a client, on a named model or the tier's newest
  tier reset [<tier>]                           Drop one tier's setting, or every tier setting and the preset

Roles: which tier, client, and effort each role runs at
  role show                                     Print each role's tier, client, model, effort, and source
  role set <role> [--tier <t>] [--client <c>] [--effort <e>]
                                                Override a role; what it leaves out stays as it was
  role reset [<role>]                           Drop one role's override, or every override
  role resolve <role> [--tier <t>] [--client <c>] [--effort <e>] [--json]
                                                Print the model the role launches on; --tier, --client,
                                                and --effort are the worker's launch choices

Tiers, strongest first: heavy (system design, architecture), high (defects, engineering, analysis),
standard (scoped work with tests), fast (classification, gathering data in volume, boilerplate).
Claude runs opus for heavy and high (fable also fits heavy), sonnet, haiku.
Codex runs its newest listed -astra, -sol, -terra, -luna model.
OpenCode runs only a model named with tier set <tier>.
heavy falls back to high when no heavy model is listed.
A review lens (review-bugs, review-spec, review-risk) follows the reviewer until it is set on its own.

The choice is stored in ~/.harness/state.json. Saving it with set, preset, reset, or the picker renders
the role agents again for each client that already has them; ` + "`harness install agents`" + ` renders them for the rest.
`

// Apply renders the role agents again from the saved choice and returns an
// exit status. The installer provides it, so this package writes no client file.
type Apply func(stdout, stderr io.Writer) int

func Command(args []string, stdout, stderr io.Writer, apply Apply) int {
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
	var saved bool
	if len(args) == 0 {
		saved, err = pickAndSave(path, stdout)
	} else {
		saved, err = run(path, args, stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness models: %v\n", err)
		return 1
	}
	if saved && apply != nil {
		if code := apply(stdout, stderr); code != 0 {
			fmt.Fprintln(stderr, "harness models: the choice is saved, but the role agents were not rendered; run `harness install agents`")
			return code
		}
	}
	return 0
}

func pickAndSave(path string, stdout io.Writer) (bool, error) {
	choice, err := Load(path)
	if err != nil {
		return false, err
	}
	final, err := tea.NewProgram(newPicker(new(Catalog), choice), tea.WithOutput(stdout), tea.WithInput(os.Stdin)).Run()
	if err != nil {
		return false, err
	}
	wrote, err := final.(picker).save(path)
	if err != nil || !wrote {
		if err == nil {
			_, err = fmt.Fprintln(stdout, "nothing written")
		}
		return false, err
	}
	_, err = run(path, []string{"show"}, stdout)
	return true, err
}

// run runs one subcommand and reports whether it saved the choice.
func run(path string, args []string, stdout io.Writer) (bool, error) {
	switch args[0] {
	case "tier":
		return tierCommand(path, args[1:], stdout)
	case "role":
		return roleCommand(path, args[1:], stdout)
	case "show":
		if len(args) > 1 {
			return false, errors.New("show takes no arguments; see `harness models tier show` and `role show`")
		}
		choice, err := Load(path)
		if err != nil {
			return false, err
		}
		cat := new(Catalog)
		showTiers(choice, cat, stdout)
		showRoles(choice, cat, stdout)
		return false, nil
	case "reset":
		if len(args) > 1 {
			return false, fmt.Errorf("reset takes no name; drop one setting with `harness models tier reset %s` or `role reset %s`", args[1], args[1])
		}
		return true, Save(path, Choice{})
	}
	return false, fmt.Errorf("unknown subcommand %q\n%s", args[0], usage)
}

// change applies one of the picker's queued edits, each spelled as the
// command that makes it: tier preset, tier set, or role set.
func change(choice Choice, args []string, cat *Catalog) (Choice, error) {
	switch strings.Join(args[:min(2, len(args))], " ") {
	case "tier preset":
		return setPreset(choice, args[2:])
	case "tier set":
		return setTier(choice, args[2:], cat)
	case "role set":
		return setRole(choice, args[2:], cat)
	}
	return choice, fmt.Errorf("unknown change %q", strings.Join(args, " "))
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
