// Command harness is the single entry point for operating the harness.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/jvillegasd/harness/internal/agents"
	"github.com/jvillegasd/harness/internal/bench"
	"github.com/jvillegasd/harness/internal/dash"
	"github.com/jvillegasd/harness/internal/install"
	"github.com/jvillegasd/harness/internal/locks"
	"github.com/jvillegasd/harness/internal/scenarios"
	"github.com/jvillegasd/harness/internal/state"
	"github.com/jvillegasd/harness/internal/statusline"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `Usage: harness <command> [arguments]

Commands:
  bootstrap  Build harness into ~/.local/bin and install it
  install    Install, reconcile, or remove the harness on this machine
  agents     Render and check the role agent definitions
  scenarios  Validate, run, and compare the artifact-based scenarios
  bench      Compare the session population before and after a harness change
  locks      Manage the reviewed third-party skill locks
  statusline Print the Claude Code status line for the payload on stdin
  state      Read and write this branch's oracle ladder and rabbit-hole counters
  dash       Show the live harness dashboard
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "bootstrap":
		return install.Bootstrap(args[1:], stdout, stderr)
	case "install":
		return install.Command(args[1:], stdout, stderr)
	case "agents":
		return agents.Command(args[1:], stdout, stderr)
	case "scenarios":
		return scenarios.Command(args[1:], stdout, stderr)
	case "bench":
		return bench.Command(args[1:], stdout, stderr)
	case "locks":
		return locks.Command(args[1:], stdout, stderr)
	case "statusline":
		return statusline.Command(args[1:], os.Stdin, stdout, stderr)
	case "state":
		return state.Command(args[1:], stdout, stderr)
	case "dash":
		return dash.Command(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "harness: unknown command %q; run harness --help\n", args[0])
	return 2
}
