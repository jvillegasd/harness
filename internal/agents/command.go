package agents

import (
	"fmt"
	"io"
)

const usageText = `Usage: harness agents [--check] <assignments.json> <output-dir>

Render every assigned role into one agent definition per client. Generation exits 0 whether or not it wrote anything, and reports whether the output changed. --check reports each output that is missing, drifted, unmanaged, or no longer carries its access level's restrictions, and exits 1 when it found any.
`

// Command runs `harness agents` and returns the exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(stdout, usageText)
		return 0
	}
	checking := len(args) > 0 && args[0] == "--check"
	rest := args
	if checking {
		rest = args[1:]
	}
	if len(rest) != 2 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	outputs, accesses, report, err := Render(rest[0])
	if err != nil {
		return report1(err, stderr)
	}
	output := rest[1]
	if code := checkSymlinks(outputs, output, stderr); code != 0 {
		return code
	}
	if checking {
		return Check(outputs, accesses, output, stdout)
	}
	return generate(outputs, output, report, stdout, stderr)
}

// GenerateCommand renders and writes the role agents for the installer, refusing a symlinked output path before anything is written.
func GenerateCommand(config, output string, stdout, stderr io.Writer) int {
	outputs, _, report, err := Render(config)
	if err != nil {
		return report1(err, stderr)
	}
	if code := checkSymlinks(outputs, output, stderr); code != 0 {
		return code
	}
	return generate(outputs, output, report, stdout, stderr)
}

// CheckCommand renders the assignments and reports drift without writing anything, for the installer's verification mode.
func CheckCommand(config, output string, stdout, stderr io.Writer) int {
	outputs, accesses, _, err := Render(config)
	if err != nil {
		return report1(err, stderr)
	}
	if code := checkSymlinks(outputs, output, stderr); code != 0 {
		return code
	}
	return Check(outputs, accesses, output, stdout)
}

func report1(err error, w io.Writer) int {
	fmt.Fprintf(w, "harness agents: %v\n", err)
	return 1
}
