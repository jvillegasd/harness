// Package skills manages the reviewed third-party skill locks: the skills.sh calls behind add, update, and remove, and an offline verification of every locked body against its recorded folder hash.
package skills

import (
	"fmt"
	"io"

	"github.com/jvillegasd/harness/internal/checkout"
)

const usage = `Usage: harness skills <command> [arguments]

Commands:
  add <owner/repo> <name>  Install and lock one upstream skill
  update                   Update every locked skill from upstream
  remove <name>            Remove one locked body, lock entry, and Claude link
  verify                   Recompute every locked body's folder hash offline
`

// Command runs `harness skills` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	sub, rest := args[0], args[1:]
	if sub == "help" || sub == "-h" || sub == "--help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	switch sub {
	case "add", "update", "remove", "verify":
	default:
		fmt.Fprintf(stderr, "harness skills: unknown command %q; run harness skills --help\n", sub)
		return 2
	}
	for _, arg := range rest {
		if arg == "-h" || arg == "--help" {
			fmt.Fprint(stdout, usage)
			return 0
		}
	}
	// The whole invocation is validated before the checkout is resolved, so a malformed one touches nothing.
	if err := validateInvocation(sub, rest); err != nil {
		fmt.Fprintf(stderr, "harness skills: %v; run harness skills --help\n", err)
		return 2
	}
	c, err := checkout.Find(".")
	if err != nil {
		fmt.Fprintf(stderr, "harness skills: %v\n", err)
		return 1
	}
	var runErr error
	switch sub {
	case "add":
		runErr = add(c, rest, stdout, stderr)
	case "update":
		runErr = update(c, stdout, stderr)
	case "remove":
		runErr = remove(c, rest, stdout, stderr)
	case "verify":
		runErr = verify(c, stdout, stderr)
	}
	if runErr != nil {
		fmt.Fprintf(stderr, "harness skills: %v\n", runErr)
		return 1
	}
	return 0
}

// validateInvocation rejects a malformed subcommand invocation without reading the checkout.
func validateInvocation(sub string, rest []string) error {
	switch sub {
	case "add":
		if len(rest) != 2 {
			return fmt.Errorf("add expects 2 argument(s)")
		}
		if err := validateSource(rest[0]); err != nil {
			return err
		}
		return validateName(rest[1])
	case "remove":
		if len(rest) != 1 {
			return fmt.Errorf("remove expects 1 argument(s)")
		}
		return validateName(rest[0])
	default:
		if len(rest) != 0 {
			return fmt.Errorf("%s expects 0 argument(s)", sub)
		}
	}
	return nil
}

// paths are the checkout-relative locations the lock tool reads and writes.
type paths struct {
	root     string
	lock     string
	skills   string
	commands string
	claude   string
}

func pathsFor(c checkout.Checkout) paths {
	return paths{
		root:     c.Root,
		lock:     c.Lock(),
		skills:   c.Skills(),
		commands: c.Commands(),
		claude:   c.ClaudeSkills(),
	}
}
