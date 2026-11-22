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
	name, component, client string

	data string
}

var tools = []tool{
	{name: "node", component: "packages"},
	{name: "gh", component: "packages"},
	{name: "uv", component: "packages"},
	{name: "claude", component: "clients", client: "claude", data: ".claude"},
	{name: "codex", component: "clients", client: "codex", data: ".codex"},
	{name: "opencode", component: "clients", client: "opencode", data: ".local/share/opencode"},
	{name: "engram", component: "mcps", data: ".engram"},
	{name: "agent-browser", component: "mcps", data: ".agent-browser"},
}

const (
	viaBrew   = "brew:"
	viaCask   = "cask:"
	viaNPM    = "npm:"
	viaNative = "native:"
)

var through = map[string]string{viaNPM: "node"}

func installedThrough(pkg string, recorded map[string]string, removing []tool) []string {
	var out []string
	for _, t := range tools {
		method, ok := recorded[t.name]
		kind, _, _ := strings.Cut(method, ":")
		if ok && through[kind+":"] == pkg && !slices.ContainsFunc(removing, func(r tool) bool { return r.name == t.name }) {
			out = append(out, t.name)
		}
	}
	return out
}

func (t tool) method() string {
	switch t.name {
	case "claude":
		return viaNative + "claude"
	case "codex":

		if runtime.GOOS == "darwin" {
			return viaCask + "codex"
		}
		return viaNPM + "@openai/codex"
	case "opencode":
		return viaBrew + "anomalyco/tap/opencode-v2"
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
	var out []tool
	for _, t := range tools {
		if slices.Contains(in.components, t.component) && (t.client == "" || slices.Contains(in.clients, t.client)) {
			out = append(out, t)
		}
	}
	return out
}

func (in *installer) installTools() {
	var missing []tool
	for _, t := range in.selectedTools() {
		if !present(t.name) {
			missing = append(missing, t)
		}
	}
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
		switch kind + ":" {
		case viaBrew, viaCask:
			if _, err := exec.LookPath("brew"); err != nil {
				die("Homebrew is required to install %s; run the root ./macos.sh or ./linux.sh first.", t.name)
			}
			if tap := filepath.Dir(target); kind+":" == viaBrew && tap != "." && !tapped[tap] {
				in.run(nil, "brew", "tap", tap)
				tapped[tap] = true
			}
			if kind+":" == viaCask {
				in.run(nil, "brew", "install", "--cask", target)
			} else {
				in.run(nil, "brew", "install", target)
			}
		case viaNPM:
			if _, err := exec.LookPath("npm"); err != nil {
				die("npm is required to install %s; run harness install packages first.", t.name)
			}
			in.run(nil, "npm", "install", "--global", target)
		case viaNative:
			in.run(nil, "bash", "-c", "set -o pipefail; curl -fsSL https://claude.ai/install.sh | bash")
		}
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
	case viaCask:
		in.run(nil, "brew", "uninstall", "--cask", target)
	case viaNPM:
		in.run(nil, "npm", "uninstall", "--global", target)
	case viaNative:

		must(os.RemoveAll(filepath.Join(in.home, ".local", "bin", "claude")))
		must(os.RemoveAll(filepath.Join(in.home, ".local", "share", "claude")))
	default:
		die("Unknown install method %q recorded for %s; fix or delete it in %s.", method, t.name, in.statePath())
	}
	in.updateState(func(s *state) { delete(s.Tools, t.name) })
}
