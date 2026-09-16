package install

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jvillegasd/harness/internal/checkout"
)

const bootstrapUsage = `Usage: harness bootstrap [--no-backup]

Build harness into ~/.local/bin and run a full harness install.

  --no-backup  Delete what is in the way instead of renaming it to
               <path>.backup.<timestamp>, and apply the same choice to the
               harness install. Destructive and unrecoverable.
`

// Bootstrap runs `harness bootstrap` with its arguments and returns the exit code.
func Bootstrap(args []string, stdout, stderr io.Writer) (code int) {
	defer recoverFatal("bootstrap", stderr, &code)
	in := &installer{stdout: stdout, stderr: stderr}
	for _, arg := range args {
		switch arg {
		case "--no-backup":
			in.noBackup = true
		case "-h", "--help":
			fmt.Fprint(stdout, bootstrapUsage)
			return 0
		default:
			fmt.Fprintf(stderr, "harness bootstrap: unknown argument %s; run harness bootstrap --help\n", arg)
			return 2
		}
	}
	dir, err := checkout.Find(".")
	if err != nil {
		die("%v", err)
	}
	in.setPaths(dir)

	bin := filepath.Join(in.home, ".local", "bin", "harness")
	in.log("Building %s", bin)
	must(os.MkdirAll(filepath.Dir(bin), 0o755))
	build := exec.Command("go", "build", "-o", bin, "./cmd/harness")
	build.Dir, build.Stdout, build.Stderr = dir.Root, stdout, stderr
	if err := build.Run(); err != nil {
		die("Building harness failed: %v", err)
	}
	in.install()
	return 0
}
