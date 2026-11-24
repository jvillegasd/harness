package agents

import (
	"fmt"
	"io"
)

func Generate(root, output string, assigned map[string]Assigned, stdout, stderr io.Writer) int {
	return generateRoles(root, output, assigned, Roles, stdout, stderr)
}

func generateRoles(root, output string, assigned map[string]Assigned, roles []Role, stdout, stderr io.Writer) int {
	outputs, _, report, err := render(root, assigned, roles)
	if err != nil {
		return report1(err, stderr)
	}
	if code := checkSymlinks(outputs, output, stderr); code != 0 {
		return code
	}
	return generate(outputs, output, report, stdout, stderr)
}

func Check(root, output string, assigned map[string]Assigned, stdout, stderr io.Writer) int {
	return checkRoles(root, output, assigned, Roles, stdout, stderr)
}

func checkRoles(root, output string, assigned map[string]Assigned, roles []Role, stdout, stderr io.Writer) int {
	outputs, accesses, _, err := render(root, assigned, roles)
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
