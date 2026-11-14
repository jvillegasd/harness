package eval

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const usage = `Usage: harness eval <command> [arguments]

Plant, run, and clean the blinded directories a skill comparison runs in.
Everything a candidate sees looks like an ordinary project; the label key and
the records live under ~/.harness/eval/<run id>/, outside every candidate.

Commands:
  plant  Create the candidate directories for a variant and a baseline skill
  clean  Delete one run's directories, key, and records
`

const plantUsage = `Usage: harness eval plant --variant <skill dir> --baseline <skill dir> --client <claude|codex|opencode> --runs <n> [--skeleton <dir>]

Create --runs directories per arm under the scratch-projects root
(~/.scratch-projects, or $HARNESS_SCRATCH_ROOT), each with an ordinary project
name, holding a copy of --skeleton and that arm's skill where the client loads
skills from its working directory: .claude/skills/<name> for claude,
.agents/skills/<name> for codex and opencode. An opencode directory also gets
an opencode.json that denies edits outside it, unless the skeleton has one.
The variant and the baseline must name the same skill. The human's installed
skills are never touched.

Print the run id. The label key, mapping each neutral label to its arm, and
the manifest of directories are written to ~/.harness/eval/<run id>/; give the
judge labels, never the key.
`

const cleanUsage = `Usage: harness eval clean <run id>

Delete every directory in the run's manifest, then its key, manifest, outputs,
and session record, and nothing else. An unknown run id exits 1 and removes
nothing.
`

// Command runs `harness eval` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "plant":
		return plant(args[1:], stdout, stderr)
	case "clean":
		return clean(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness eval: unknown command %q; run harness eval --help\n", args[0])
	return 2
}

func parse(flags *flag.FlagSet, args []string, text string, stdout, stderr io.Writer) int {
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, text)
			return 0
		}
		fmt.Fprintf(stderr, "harness eval %s: %v\n%s", flags.Name(), err, text)
		return 2
	}
	return -1
}

func home(name string, stderr io.Writer) (string, bool) {
	dir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "harness eval %s: %v\n", name, err)
		return "", false
	}
	return dir, true
}

func plant(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("plant", flag.ContinueOnError)
	var o PlantOptions
	flags.StringVar(&o.Variant, "variant", "", "")
	flags.StringVar(&o.Baseline, "baseline", "", "")
	flags.StringVar(&o.Skeleton, "skeleton", "", "")
	flags.StringVar(&o.Client, "client", "", "")
	flags.IntVar(&o.Runs, "runs", 0, "")
	if code := parse(flags, args, plantUsage, stdout, stderr); code >= 0 {
		return code
	}
	switch {
	case flags.NArg() != 0:
		fmt.Fprintf(stderr, "harness eval plant: unexpected argument %q\n", flags.Arg(0))
		return 2
	case o.Variant == "" || o.Baseline == "":
		fmt.Fprintln(stderr, "harness eval plant: --variant and --baseline are required; run harness eval plant --help")
		return 2
	case skillPaths[o.Client] == "":
		fmt.Fprintf(stderr, "harness eval plant: --client must be claude, codex, or opencode, not %q\n", o.Client)
		return 2
	case o.Runs < 1:
		fmt.Fprintln(stderr, "harness eval plant: --runs must be at least 1")
		return 2
	}
	h, ok := home("plant", stderr)
	if !ok {
		return 1
	}
	for _, p := range []*string{&o.Variant, &o.Baseline, &o.Skeleton} {
		if *p != "" {
			abs, err := filepath.Abs(*p)
			if err != nil {
				fmt.Fprintf(stderr, "harness eval plant: %v\n", err)
				return 1
			}
			*p = abs
		}
	}
	m, err := Plant(h, o)
	if err != nil {
		fmt.Fprintf(stderr, "harness eval plant: %v\n", err)
		if m.Run != "" {
			fmt.Fprintf(stderr, "harness eval plant: run harness eval clean %s to remove what was planted\n", m.Run)
		}
		return 1
	}
	fmt.Fprintln(stdout, m.Run)
	return 0
}

func clean(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("clean", flag.ContinueOnError)
	if code := parse(flags, args, cleanUsage, stdout, stderr); code >= 0 {
		return code
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "harness eval clean: name one run id; run harness eval clean --help")
		return 2
	}
	h, ok := home("clean", stderr)
	if !ok {
		return 1
	}
	removed, err := Clean(h, flags.Arg(0))
	for _, path := range removed {
		fmt.Fprintln(stdout, "removed "+path)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness eval clean: %v\n", err)
		return 1
	}
	return 0
}
