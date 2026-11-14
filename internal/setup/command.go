package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const usage = `Usage: harness setup <command> [arguments]

Gather the evidence a repository's tracker mode is chosen from, and write the
chosen mode. Choosing the mode stays with setup-harness and the human.

Commands:
  probe    Print the tracker-mode evidence as JSON, changing nothing
  tracker  Write the chosen tracker mode's configuration and automation
`

const probeUsage = `Usage: harness setup probe

Print, as JSON, the evidence the tracker mode is chosen from: whether the
repository has a GitHub remote, its default branch and whether that branch is
protected (null, with protected_error, when the forge could not say), the mode
docs/agents/harness.json already selects, and any local spec tree. It changes
nothing.
`

const trackerUsage = `Usage: harness setup tracker --mode <github|local-markdown> [--spec-home <dir>]

Create or update docs/agents/harness.json in its stable shape, keeping mapped
label values, criticalPaths, and keys it does not know, and print every path
or label it created, updated, or removed. A second run with the same arguments
changes nothing and prints nothing.

github          Verify forge credentials with gh auth status before any write,
                create the mapped labels the repository lacks, install
                .github/workflows/agent-lifecycle.yml and
                .github/scripts/reconcile-agent-lifecycle.go from the harness's
                copies, and remove the superseded reconcile-agent-lifecycle.py.
local-markdown  Set the two mode values, with --spec-home (default
                docs/specs); create no label and install no workflow.
`

// Command runs `harness setup` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "probe":
		return probe(args[1:], stdout, stderr)
	case "tracker":
		return tracker(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness setup: unknown command %q; run harness setup --help\n", args[0])
	return 2
}

func parse(flags *flag.FlagSet, args []string, text string, stdout, stderr io.Writer) int {
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, text)
			return 0
		}
		fmt.Fprintf(stderr, "harness setup %s: %v\n%s", flags.Name(), err, text)
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "harness setup %s: unexpected argument %q; run harness setup %s --help\n", flags.Name(), flags.Arg(0), flags.Name())
		return 2
	}
	return -1
}

func root(name string, stderr io.Writer) (string, bool) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		fmt.Fprintf(stderr, "harness setup %s: not inside a git repository\n", name)
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// gh is the Forge that runs the gh CLI in dir.
func gh(dir string) Forge {
	return func(args ...string) ([]byte, error) {
		cmd := exec.Command("gh", args...)
		cmd.Dir = dir
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			return out.Bytes(), fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
		}
		return out.Bytes(), nil
	}
}

func probe(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	if code := parse(flags, args, probeUsage, stdout, stderr); code >= 0 {
		return code
	}
	top, ok := root("probe", stderr)
	if !ok {
		return 2
	}
	out, _ := json.MarshalIndent(Probe(top, gh(top)), "", "  ")
	fmt.Fprintln(stdout, string(out))
	return 0
}

func tracker(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("tracker", flag.ContinueOnError)
	var o Options
	flags.StringVar(&o.Mode, "mode", "", "")
	flags.StringVar(&o.SpecHome, "spec-home", "", "")
	if code := parse(flags, args, trackerUsage, stdout, stderr); code >= 0 {
		return code
	}
	switch {
	case o.Mode != "github" && o.Mode != "local-markdown":
		fmt.Fprintf(stderr, "harness setup tracker: --mode must be github or local-markdown, not %q\n", o.Mode)
		return 2
	case o.Mode == "github" && o.SpecHome != "":
		fmt.Fprintln(stderr, "harness setup tracker: --spec-home applies only to local-markdown")
		return 2
	}
	top, ok := root("tracker", stderr)
	if !ok {
		return 2
	}
	done, err := Tracker(top, o, gh(top))
	for _, line := range done {
		fmt.Fprintln(stdout, line)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness setup tracker: %v\n", err)
		return 1
	}
	return 0
}
