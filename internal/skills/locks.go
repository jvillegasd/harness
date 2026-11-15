package skills

import (
	"fmt"
	"io"
	"strings"

	"github.com/jvillegasd/harness/internal/checkout"
)

const usage = `Usage: harness skills <command> [arguments]

Commands:
  add <owner/repo> <name> <group>
                           Install and lock one upstream skill in skills/<group>/
  update                   Update every locked skill from upstream
  remove <name>            Remove one locked body, lock entry, and Claude link
  verify                   Recompute every locked body's folder hash offline
  new <name> <group> [--guarded]
                           Create a local skill in skills/<group>/ with a
                           starting body and both links; --guarded adds the
                           three invocation guards, one per client
  move <name> <group>      Move a skill's body to another group with git mv
                           and repoint its link
`

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
	case "add", "update", "remove", "verify", "new", "move":
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
	case "new":
		runErr = newSkill(c, rest, stdout)
	case "move":
		runErr = moveSkill(c, rest, stdout)
	}
	if runErr != nil {
		fmt.Fprintf(stderr, "harness skills: %v\n", runErr)
		return 1
	}
	return 0
}

func validateInvocation(sub string, rest []string) error {
	switch sub {
	case "add":
		if len(rest) != 3 {
			return fmt.Errorf("add expects 3 argument(s)")
		}
		if err := validateSource(rest[0]); err != nil {
			return err
		}
		if err := validateName(rest[1]); err != nil {
			return err
		}
		return validateName(rest[2])
	case "new", "move":
		var names []string
		for _, a := range rest {
			if sub == "new" && a == "--guarded" {
				continue
			}
			if strings.HasPrefix(a, "-") {
				return fmt.Errorf("unknown flag %s", a)
			}
			names = append(names, a)
		}
		if len(names) != 2 {
			return fmt.Errorf("%s expects <name> <group>", sub)
		}
		if err := validateName(names[0]); err != nil {
			return err
		}
		if err := validateName(names[1]); err != nil {
			return fmt.Errorf("invalid group name: %s", names[1])
		}
		return nil
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

type paths struct {
	root   string
	lock   string
	skills string
	agents string
	claude string
}

func pathsFor(c checkout.Checkout) paths {
	return paths{
		root:   c.Root,
		lock:   c.Lock(),
		skills: c.Skills(),
		agents: c.AgentSkills(),
		claude: c.ClaudeSkills(),
	}
}
