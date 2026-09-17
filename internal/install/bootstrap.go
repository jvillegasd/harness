package install

import (
	"fmt"
	"io"
)

const bootstrapUsage = `Usage: harness bootstrap [--no-backup]

Copy this harness binary into ~/.local/bin and run a full harness install,
the same as harness install.

  --no-backup  Delete what is in the way instead of renaming it to
               <path>.backup.<timestamp>, and apply the same choice to the
               harness install. Destructive and unrecoverable.
`

// Bootstrap runs `harness bootstrap` with its arguments and returns the exit code.
func Bootstrap(args []string, stdout, stderr io.Writer) (code int) {
	defer recoverFatal("bootstrap", stderr, &code)
	in := &installer{stdout: stdout, stderr: stderr, command: "bootstrap"}
	for _, arg := range args {
		switch arg {
		case "--no-backup":
			in.noBackup = true
		case "-h", "--help":
			fmt.Fprint(stdout, bootstrapUsage)
			return 0
		default:
			fmt.Fprintf(stderr, "harness bootstrap: unknown argument %q; run harness bootstrap --help\n", arg)
			return 2
		}
	}
	in.resolveLocations()
	in.dir = in.dataRoot()
	in.install()
	return 0
}
