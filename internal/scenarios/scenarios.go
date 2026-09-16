// Package scenarios runs artifact-based scenarios for critical harness behavior, replacing the Python runner.
//
// Each scenario declares the artifacts and assertions it expects before any model session runs, executes in a disposable repository with a forge shim on PATH, and retains one attempt directory holding prompts, input provenance, tool events, artifact results, and transcript evidence. An attempt passes only when its artifact checks pass and a transcript review record exists. Changed scenario or harness inputs mark an older attempt stale.
//
// Paid model execution is off unless --allow-paid is passed with a retained per-batch session cap. Local tooling success is reported separately from behavioral acceptance.
package scenarios

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jvillegasd/harness/internal/checkout"
)

var requiredEvidence = []string{
	"provenance.json",
	"prompts.txt",
	"events.jsonl",
	"artifacts.json",
	"transcript.jsonl",
	"metrics.json",
}

var artifactKeys = []string{"tree_unchanged", "fixtures_unchanged", "present", "absent", "contains"}

var stateOrder = []string{"pass", "fail", "interrupted", "stale", "unreviewed", "incomplete", "unavailable"}

const batchFile = "batch.json"

const usageText = `Usage: harness scenarios <command> [flags]

Commands:
  validate   Reject the matrix before any session runs
  run        Execute the declared scenarios and retain one attempt each
  report     Report local tooling and behavioral acceptance separately
  compare    Compare two retained batches, pass rates before efficiency

Flags:
  --matrix <path>   Matrix declaration (default <root>/scenarios/matrix.json)
  --out <path>      Attempt directory (default <root>/scenarios/attempts)
  --root <path>     Harness checkout the harness_inputs resolve against (default: the
                    enclosing checkout, required outside one)
  run:     --scenario <id> --fixtures <path> --allow-paid --max-sessions <n>
  compare: --before <dir> --after <dir>
`

// Command runs `harness scenarios` and returns the exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, usageText)
		return 0
	}
	sub, rest := args[0], args[1:]
	flags := newFlags()
	matrix := flags.String("matrix", "", "")
	out := flags.String("out", "", "")
	rootFlag := flags.String("root", "", "")
	var scenario, fixtures, before, after *string
	var allowPaid *bool
	var maxSessions *int
	switch sub {
	case "validate", "report":
	case "run":
		scenario = flags.String("scenario", "", "")
		fixtures = flags.String("fixtures", "", "")
		allowPaid = flags.Bool("allow-paid", false, "")
		maxSessions = flags.Int("max-sessions", 0, "")
	case "compare":
		before = flags.String("before", "", "")
		after = flags.String("after", "", "")
	default:
		fmt.Fprintf(stderr, "harness scenarios: unknown command %q; run harness scenarios --help\n", sub)
		return 2
	}
	for _, arg := range rest {
		if arg == "-h" || arg == "--help" {
			fmt.Fprint(stdout, usageText)
			return 0
		}
	}
	if err := flags.Parse(rest); err != nil {
		fmt.Fprintf(stderr, "harness scenarios: %v; run harness scenarios --help\n", err)
		return 2
	}
	// The invocation is validated before the checkout is resolved or the matrix is read, so a malformed one touches nothing.
	switch sub {
	case "compare":
		if *before == "" || *after == "" {
			fmt.Fprintln(stderr, "harness scenarios: compare needs --before and --after")
			return 2
		}
	case "run":
		if *allowPaid != isFlagSet(flags, "max-sessions") {
			fmt.Fprintln(stderr, "harness scenarios: run takes --allow-paid and --max-sessions together; a paid batch retains its cap.")
			return 2
		}
	}
	root := *rootFlag
	if root == "" {
		// The default is the enclosing checkout, not the invocation directory, so the matrix, attempts, and fixtures resolve from a nested directory the same way every other checkout-dependent command resolves them.
		c, err := checkout.Find(".")
		if err != nil {
			fmt.Fprintf(stderr, "harness scenarios: %v\n", err)
			return 1
		}
		root = c.Root
	}
	if *matrix == "" {
		*matrix = filepath.Join(root, "scenarios", "matrix.json")
	}
	if *out == "" {
		*out = filepath.Join(root, "scenarios", "attempts")
	}
	data, err := os.ReadFile(*matrix)
	if err != nil {
		fmt.Fprintf(stderr, "harness scenarios: %v\n", err)
		return 1
	}
	decoded, err := decodeJSON(data)
	if err != nil {
		fmt.Fprintf(stderr, "harness scenarios: %s: %v\n", *matrix, err)
		return 1
	}
	declaration, _ := decoded.(map[string]any)
	switch sub {
	case "validate":
		errors := validateMatrix(declaration)
		for _, problem := range errors {
			fmt.Fprintln(stderr, problem)
		}
		if len(errors) > 0 {
			fmt.Fprintln(stdout, "local-tooling: fail")
			return 1
		}
		fmt.Fprintln(stdout, "local-tooling: pass")
		return 0
	case "report":
		fmt.Fprintln(stdout, report(declaration, *out, root))
		return 0
	case "compare":
		fmt.Fprintln(stdout, compare(*before, *after, declaration, root))
		return 0
	case "run":
		if *scenario != "" && !hasScenario(declaration, *scenario) {
			fmt.Fprintf(stderr, "harness scenarios: unknown scenario %q; run harness scenarios --help\n", *scenario)
			return 2
		}
		fixturesPath := *fixtures
		if fixturesPath == "" {
			fixturesPath = filepath.Join(root, "scenarios", "fixtures")
		}
		var cap *int
		if isFlagSet(flags, "max-sessions") {
			cap = maxSessions
		}
		errors := validateMatrix(declaration)
		if len(errors) > 0 {
			for _, problem := range errors {
				fmt.Fprintln(stderr, problem)
			}
			return 1
		}
		exitCode := 0
		for _, raw := range list(declaration, "scenarios") {
			entry, _ := raw.(map[string]any)
			if *scenario != "" && getString(entry, "id") != *scenario {
				continue
			}
			state, where, err := runOne(entry, root, *out, fixturesPath, *allowPaid, cap)
			if err != nil {
				fmt.Fprintf(stderr, "%s: %v\n", getString(entry, "id"), err)
				exitCode = 1
				continue
			}
			fmt.Fprintf(stdout, "%s: %s -> %s\n", getString(entry, "id"), state, where)
			if state == "fail" || state == "not-run" || state == "interrupted" {
				exitCode = 1
			}
		}
		fmt.Fprintln(stdout, report(declaration, *out, root))
		return exitCode
	}
	return 2
}

// hasScenario reports whether the declaration names a scenario with this id, so a narrowed run can reject an id that matches nothing.
func hasScenario(declaration map[string]any, id string) bool {
	for _, raw := range list(declaration, "scenarios") {
		entry, _ := raw.(map[string]any)
		if getString(entry, "id") == id {
			return true
		}
	}
	return false
}
