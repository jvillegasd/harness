package land

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/jvillegasd/harness/internal/statefile"
)

const prUsage = `Usage: harness pr <command> [arguments]

Commands:
  status  Report whether pull requests are ready to merge, and what blocks them
`

const statusUsage = `Usage: harness pr status [--stack] [<pr>...]

Print, as a JSON array in landing order, whether each pull request is ready to
merge and what stands in the way: the numbers given, or with none the pull
request of the branch checked out here. It reads the forge through gh and
fetches each pull request's head and base from origin; it writes nothing to
the forge and creates no local branch.

  --stack  The one number is a stack's tip: every open pull request under it
           is reported, bottom-up.

Each verdict carries:

  ready     true when nothing holds the merge
  blocker   the first blocker: conflict, thread, failing, pending, or other
  review    the review coverage, as harness land check reports it
  findings  the ledger's open findings and recorded dismissals
  threads   every unresolved review thread: id, path, line, author, the
            author's permission on the base repository, the first comment's
            text, and its url
  holds     every reason not to merge, the same as harness land check --merge
            prints for the same head
  notes     findings that hold nothing

The exit code names the first pull request's first blocker, in the order to
work them: 0 ready, 3 merge conflict, 4 unresolved review thread, 5 failing
required check, 6 pending required check, 7 any other hold (a draft, a merged
or closed pull request, a missing approval, a required review that is not
current, an open ledger finding). 1 is an operational failure and 2 an invalid
invocation.
`

// PrCommand runs `harness pr` and returns the process exit code.
func PrCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, prUsage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, prUsage)
		return 0
	case "status":
		return status(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness pr: unknown command %q; run harness pr --help\n", args[0])
	return 2
}

func status(args []string, stdout, stderr io.Writer) int {
	var numbers []int
	stack := false
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprint(stdout, statusUsage)
			return 0
		case "--stack":
			stack = true
		default:
			n, err := strconv.Atoi(a)
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "harness pr status: %q is not a pull request number or a flag; run harness pr status --help\n", a)
				return 2
			}
			numbers = append(numbers, n)
		}
	}
	if stack && len(numbers) != 1 {
		fmt.Fprintln(stderr, "harness pr status: --stack takes exactly one pull request number, the stack's tip")
		return 2
	}
	e, err := cwdEnv()
	if err != nil {
		fmt.Fprintf(stderr, "harness pr status: %v\n", err)
		return 1
	}
	statuses, err := Statuses(e, numbers, stack)
	if err != nil {
		fmt.Fprintf(stderr, "harness pr status: %v\n", err)
		return 1
	}
	out, _ := json.MarshalIndent(statuses, "", "  ")
	fmt.Fprintln(stdout, string(out))
	return Code(statuses)
}

// cwdEnv is the Env for the repository the command runs in, reading the forge through gh.
func cwdEnv() (Env, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Env{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	return Env{Dir: cwd, Forge: Gh(cwd), State: statefile.Path(home)}, nil
}
