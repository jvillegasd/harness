package install

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/ui"
)

// ApplyAgents renders the role agents again for each client whose agents the
// harness already installed, so a model choice reaches them without a full
// install. It installs nothing new and leaves every other component alone.
func ApplyAgents(stdout, stderr io.Writer) (code int) {
	defer recoverFatal("models", stderr, &code)
	in := &installer{verb: "models", stdout: stdout, stderr: stderr, quietReport: true, keepEdited: true}
	in.resolveLocations()
	installed := in.loadState().Installed
	for _, client := range allClients {
		if slices.Contains(installed[client], "agents") && present(client) {
			in.clients = append(in.clients, client)
		}
	}
	if len(in.clients) == 0 {
		fmt.Fprintln(stdout, "No client has the harness's role agents yet; `harness install agents` renders them with this choice.")
		return 0
	}
	in.components = []string{"agents"}
	defer in.extractAssets()()
	in.installFiles()
	in.checkFiles()
	fmt.Fprintln(stdout, ui.For(stdout).OK(fmt.Sprintf("Applied to the role agents of %s.", strings.Join(in.clients, ", "))))
	return 0
}
