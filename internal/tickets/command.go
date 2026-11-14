package tickets

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

const usage = `Usage: harness tickets <command> [arguments]

Work with the ticket drafts under docs/agents/tickets/<feature-slug>/.

Commands:
  check     Validate the drafts of one feature, or of every feature
`

const checkUsage = `Usage: harness tickets check [<feature-slug>]

Validate the ticket drafts in docs/agents/tickets/<feature-slug>/, or in every
feature directory when none is named, and print each problem as
path:line: message. Exit 0 when there is none and 1 otherwise.

A draft is a file named <NN>-<slug>.md holding a title line (# <NN> <title>)
and five non-empty sections: ## What to build, ## Acceptance criteria,
## Seam, ## Blocked by, and ## Out of scope. Blocked by holds none or the
numbers of tickets in the same feature, numbers are unique within a feature,
and the blocking edges form no cycle.
`

// Command runs `harness tickets` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "check":
		return check(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness tickets: unknown command %q; run harness tickets --help\n", args[0])
	return 2
}

// parse handles help and usage errors before any read, returning -1 to continue.
func parse(flags *flag.FlagSet, args []string, text string, stdout, stderr io.Writer) int {
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, text)
			return 0
		}
		fmt.Fprintf(stderr, "harness tickets %s: %v\n%s", flags.Name(), err, text)
		return 2
	}
	return -1
}

// root returns the top of the checkout holding the working directory.
func root(name string, stderr io.Writer) (string, bool) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		fmt.Fprintf(stderr, "harness tickets %s: not inside a git repository\n", name)
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// slug validates a feature slug so it names one directory under the draft directory.
func slug(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid feature slug %q", name)
	}
	return nil
}

func check(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	if code := parse(flags, args, checkUsage, stdout, stderr); code >= 0 {
		return code
	}
	if flags.NArg() > 1 {
		fmt.Fprintf(stderr, "harness tickets check: unexpected argument %q; run harness tickets check --help\n", flags.Arg(1))
		return 2
	}
	if flags.NArg() == 1 {
		if err := slug(flags.Arg(0)); err != nil {
			fmt.Fprintf(stderr, "harness tickets check: %v\n", err)
			return 2
		}
	}
	top, ok := root("check", stderr)
	if !ok {
		return 2
	}
	features := []string{flags.Arg(0)}
	if flags.NArg() == 0 {
		var err error
		if features, err = Features(top); err != nil {
			fmt.Fprintf(stderr, "harness tickets check: %v\n", err)
			return 1
		}
	}
	found := false
	for _, f := range features {
		_, problems, err := Check(top, filepath.Join(DraftDir, f))
		if err != nil {
			fmt.Fprintf(stderr, "harness tickets check: %v\n", err)
			return 1
		}
		for _, p := range problems {
			fmt.Fprintln(stdout, p)
			found = true
		}
	}
	if found {
		return 1
	}
	return 0
}
