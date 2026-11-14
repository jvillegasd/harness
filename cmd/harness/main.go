// Command harness is the single entry point for operating the harness.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/jvillegasd/harness/internal/bench"
	"github.com/jvillegasd/harness/internal/changemap"
	"github.com/jvillegasd/harness/internal/install"
	"github.com/jvillegasd/harness/internal/land"
	"github.com/jvillegasd/harness/internal/models"
	"github.com/jvillegasd/harness/internal/resume"
	"github.com/jvillegasd/harness/internal/reviewledger"
	"github.com/jvillegasd/harness/internal/skills"
	"github.com/jvillegasd/harness/internal/statusline"
	"github.com/jvillegasd/harness/internal/update"
	"github.com/jvillegasd/harness/internal/worktree"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `Usage: harness <command> [arguments]

Commands:
  install    Install or reconcile the harness components on this machine
  update     Update installed tools or locked upstream skills explicitly
  uninstall  Remove harness components, and with --all the harness itself
  bench      Compare the session population before and after a harness change
  skills     Manage the reviewed third-party skill locks
  statusline Print the Claude Code status line for the payload on stdin
  models     Choose each delegated role's client, model, and effort
  change-map Classify this branch's changed files into review tiers
  review     Record and read the local review ledger
  worktree   Audit and retire linked worktrees without losing work
  land       Report whether pull requests can land, and how
  pr         Report a pull request's readiness and what blocks it
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "install":
		return install.Command(args[1:], stdout, stderr)
	case "update":
		return update.Command(args[1:], stdout, stderr)
	case "uninstall":
		return install.UninstallCommand(args[1:], stdout, stderr)
	case "bench":
		return bench.Command(args[1:], stdout, stderr)
	case "skills":
		return skills.Command(args[1:], stdout, stderr)
	case "statusline":
		return statusline.Command(args[1:], os.Stdin, stdout, stderr)
	case "models":
		return models.Command(args[1:], stdout, stderr)
	case "change-map":
		return changemap.Command(args[1:], stdout, stderr)
	case "review":
		return reviewledger.Command(args[1:], stdout, stderr)
	case "worktree":
		return worktree.Command(args[1:], stdout, stderr)
	case "land":
		return land.Command(args[1:], stdout, stderr)
	case "pr":
		return land.PrCommand(args[1:], stdout, stderr)
	case "resume":
		return resume.Command(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "harness: unknown command %q; run harness --help\n", args[0])
	return 2
}
