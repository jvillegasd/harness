package agents

import (
	"fmt"
	"io"
)

// Generate renders and writes the role agents for the installer, refusing a symlinked output path before anything is written.
func Generate(config, output string, stdout, stderr io.Writer) int {
	outputs, _, report, err := Render(config)
	if err != nil {
		return report1(err, stderr)
	}
	if code := checkSymlinks(outputs, output, stderr); code != 0 {
		return code
	}
	return generate(outputs, output, report, stdout, stderr)
}

// Check renders the assignments and reports drift without writing anything, for the installer's verification mode.
func Check(config, output string, stdout, stderr io.Writer) int {
	outputs, accesses, _, err := Render(config)
	if err != nil {
		return report1(err, stderr)
	}
	if code := checkSymlinks(outputs, output, stderr); code != 0 {
		return code
	}
	return checkOutputs(outputs, accesses, output, stdout)
}

func report1(err error, w io.Writer) int {
	fmt.Fprintf(w, "agents: %v\n", err)
	return 1
}
