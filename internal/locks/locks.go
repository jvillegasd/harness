// Package locks manages the reviewed third-party skill locks: the skills.sh calls behind add, update, and remove, the repository hygiene checks, and an offline verification of every locked body against its recorded folder hash.
package locks

import (
	"fmt"
	"io"

	"github.com/jvillegasd/harness/internal/checkout"
)

const usage = `Usage: harness locks <command> [arguments]

Commands:
  add <owner/repo> <name>  Install and lock one upstream skill
  update                   Update every locked skill from upstream
  remove <name>            Remove one locked body, lock entry, and Claude link
  check                    Run shell syntax and diff hygiene checks
  verify                   Recompute every locked body's folder hash offline
`

// Command runs `harness locks` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "add", "update", "remove", "check", "verify":
	default:
		fmt.Fprintf(stderr, "harness locks: unknown command %q; run harness locks --help\n", args[0])
		return 2
	}
	c, err := checkout.Find(".")
	if err != nil {
		fmt.Fprintf(stderr, "[err] %v\n", err)
		return 1
	}
	var runErr error
	switch args[0] {
	case "add":
		runErr = add(c, args[1:], stdout, stderr)
	case "update":
		runErr = update(c, args[1:], stdout, stderr)
	case "remove":
		runErr = remove(c, args[1:], stdout, stderr)
	case "check":
		runErr = check(c, args[1:], stdout, stderr)
	case "verify":
		runErr = verify(c, args[1:], stdout, stderr)
	}
	if runErr != nil {
		fmt.Fprintf(stderr, "[err] %v\n", runErr)
		return 1
	}
	return 0
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
