package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type tool struct {
	name, component string

	data string
}

var tools = []tool{
	{name: "node", component: "packages"},
	{name: "gh", component: "packages"},
	{name: "uv", component: "packages"},
	{name: "engram", component: "mcps", data: ".engram"},
	{name: "agent-browser", component: "mcps", data: ".agent-browser"},
}

const viaBrew = "brew:"

// clientTools are the clients an earlier harness installed. The dotfiles install them now, so the harness forgets their records and never removes them.
var clientTools = []string{"claude", "codex", "opencode"}

func (t tool) method() string {
	switch t.name {
	case "engram":
		return viaBrew + engramFormula
	case "agent-browser":
		return viaBrew + agentBrowserFormula
	}
	return viaBrew + t.name
}

func present(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (in *installer) usePATH() {
	must(os.Setenv("PATH", filepath.Join(in.home, ".local", "bin")+string(os.PathListSeparator)+os.Getenv("PATH")))
}

func (in *installer) selectedTools() []tool {
	return slices.DeleteFunc(slices.Clone(tools), func(t tool) bool {
		return !slices.Contains(in.components, t.component)
	})
}

func (in *installer) installTools() {
	missing := slices.DeleteFunc(in.selectedTools(), func(t tool) bool { return present(t.name) })
	if len(missing) == 0 {
		return
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		die("The harness installer supports macOS and Linux, not %s.", runtime.GOOS)
	}
	tapped := map[string]bool{}
	for _, t := range missing {
		method := t.method()
		kind, target, _ := strings.Cut(method, ":")
		in.log("Installing %s", t.name)
		if kind+":" != viaBrew {
			die("Unknown install method %q for %s.", method, t.name)
		}
		if _, err := exec.LookPath("brew"); err != nil {
			die("Homebrew is required to install %s; run the root ./macos.sh or ./linux.sh first.", t.name)
		}
		if tap := filepath.Dir(target); tap != "." && !tapped[tap] {
			in.run(nil, "brew", "tap", tap)
			tapped[tap] = true
		}
		in.run(nil, "brew", "install", target)
		in.updateState(func(s *state) { s.Tools[t.name] = method })

		if t.name == "agent-browser" {
			in.log("Downloading Chrome for Testing for agent-browser")
			if runtime.GOOS == "darwin" {
				in.run(nil, "agent-browser", "install")
			} else {
				in.run(nil, "agent-browser", "install", "--with-deps")
			}
		}
	}
}

func (in *installer) removeTool(t tool, method string) {
	kind, target, _ := strings.Cut(method, ":")

	if !present(t.name) {
		in.log("%s is already gone; dropping its record", t.name)
		in.updateState(func(s *state) { delete(s.Tools, t.name) })
		return
	}
	in.log("Removing %s", t.name)
	switch kind + ":" {
	case viaBrew:
		in.run(nil, "brew", "uninstall", target)
	default:
		die("Unknown install method %q recorded for %s; fix or delete it in %s.", method, t.name, in.statePath())
	}
	in.updateState(func(s *state) { delete(s.Tools, t.name) })
}
