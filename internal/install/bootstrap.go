package install

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const bootstrapUsage = `Usage: harness bootstrap [--no-backup]

Build harness into ~/.local/bin and run a full harness install.

  --no-backup  Delete what is in the way instead of renaming it to
               <path>.backup.<timestamp>, and apply the same choice to the
               harness install. Destructive and unrecoverable.
`

// Bootstrap runs `harness bootstrap` with its arguments and returns the exit code.
func Bootstrap(args []string, stdout, stderr io.Writer) (code int) {
	defer recoverFatal(stderr, &code)
	in := &installer{stdout: stdout, stderr: stderr}
	for _, arg := range args {
		switch arg {
		case "--no-backup":
			in.noBackup = true
		case "-h", "--help":
			fmt.Fprint(stdout, bootstrapUsage)
			return 0
		default:
			die("Unknown argument: %s", arg)
		}
	}
	checkout := findCheckout()
	in.setPaths(checkout)

	bin := filepath.Join(in.home, ".local", "bin", "harness")
	in.log("Building %s", bin)
	must(os.MkdirAll(filepath.Dir(bin), 0o755))
	build := exec.Command("go", "build", "-o", bin, "./cmd/harness")
	build.Dir, build.Stdout, build.Stderr = checkout, stdout, stderr
	if err := build.Run(); err != nil {
		die("Building harness failed: %v", err)
	}
	in.install()
	return 0
}
