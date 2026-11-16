package agents

import (
	"fmt"
	"io"
)

func Generate(root, output string, assigned map[string]Assigned, stdout, stderr io.Writer) int {
	outputs, _, report, err := Render(root, assigned)
	if err != nil {
		return report1(err, stderr)
	}
	if code := checkSymlinks(outputs, output, stderr); code != 0 {
		return code
	}
	return generate(outputs, output, report, stdout, stderr)
}

func Check(root, output string, assigned map[string]Assigned, stdout, stderr io.Writer) int {
	outputs, accesses, _, err := Render(root, assigned)
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
