package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// tool is one program a component puts on the machine. client is the client it belongs to, or empty for a tool every client shares.
type tool struct {
	name, component, client string
	// data is the directory the tool keeps its user's data in, which no uninstall deletes.
	data string
}

// tools in the order an install adds them: packages first, since a client may install through one (Codex through npm on Linux), then the clients, then the MCP servers the clients register. An uninstall walks it backwards.
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

// A recorded method names how a tool was installed and so how it is removed.
const (
	viaBrew   = "brew:"   // brew:<formula>
	viaCask   = "cask:"   // cask:<token>
	viaNPM    = "npm:"    // npm:<package>
	viaNative = "native:" // native:claude, Anthropic's installer
)

// method is how this machine installs a missing tool.
func (t tool) method() string {
	switch t.name {
	case "claude":
		return viaNative + "claude"
	case "codex":
		// Homebrew on Linux takes no casks, and Codex has no formula, so Linux gets OpenAI's npm package.
		if runtime.GOOS == "darwin" {
			return viaCask + "codex"
		}
		return viaNPM + "@openai/codex"
	case "opencode":
		return viaBrew + "anomalyco/tap/opencode"
	case "engram":
		return viaBrew + engramFormula
	case "agent-browser":
		return viaBrew + agentBrowserFormula
	}
	return viaBrew + t.name
}

// present reports whether a tool is on PATH, however it got there. A tool already present is never installed over, so its version and source stay the user's.
func present(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// usePATH puts ~/.local/bin first on PATH, where this installer and a prior run's Claude Code put their binaries, so a presence check sees them.
func (in *installer) usePATH() {
	must(os.Setenv("PATH", filepath.Join(in.home, ".local", "bin")+string(os.PathListSeparator)+os.Getenv("PATH")))
}

// selectedTools lists the tools of the selected components that serve the target clients: a client's own tool only for that client, a shared one for any.
func (in *installer) selectedTools() []tool {
	var out []tool
	for _, t := range tools {
		if slices.Contains(in.components, t.component) && (t.client == "" || slices.Contains(in.clients, t.client)) {
			out = append(out, t)
		}
	}
	return out
}

// installTools adds each selected tool missing from PATH and records how, and touches Homebrew and the network only for a tool that is missing.
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
		// The formula's caveat, run rather than printed: without the browser every call through the MCP fails mid-session. Linux needs its system libraries too.
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

// removeTool uninstalls a recorded tool the way it was installed and drops its record. A tool's data directory stays.
func (in *installer) removeTool(t tool, method string) {
	kind, target, _ := strings.Cut(method, ":")
	// Already gone, as after a removal by hand: only the record is left to drop.
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
		// What Anthropic's installer writes: the launcher and the versions it points into. ~/.claude is the user's data and stays.
		must(os.RemoveAll(filepath.Join(in.home, ".local", "bin", "claude")))
		must(os.RemoveAll(filepath.Join(in.home, ".local", "share", "claude")))
	default:
		die("Unknown install method %q recorded for %s; fix or delete it in %s.", method, t.name, in.statePath())
	}
	in.updateState(func(s *state) { delete(s.Tools, t.name) })
}
