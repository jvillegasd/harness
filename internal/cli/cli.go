// Package cli holds the flag parsing shared by every harness subcommand.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// Parse parses args into flags, allowing positionals between flags, and
// returns the positionals. A code of -1 means continue; any other code is the
// exit status after help or an error was printed.
func Parse(prefix string, flags *flag.FlagSet, args []string, usage string, stdout, stderr io.Writer) (positional []string, code int) {
	flags.SetOutput(io.Discard)
	for {
		if err := flags.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fmt.Fprint(stdout, usage)
				return nil, 0
			}
			fmt.Fprintf(stderr, "%s %s: %v\n%s", prefix, flags.Name(), err, usage)
			return nil, 2
		}
		if consumed := len(args) - flags.NArg(); consumed > 0 && args[consumed-1] == "--" {
			return append(positional, flags.Args()...), -1
		}
		if flags.NArg() == 0 {
			return positional, -1
		}
		positional = append(positional, flags.Arg(0))
		args = flags.Args()[1:]
	}
}
