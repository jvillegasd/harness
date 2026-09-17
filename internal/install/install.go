// Package install copies the harness the binary carries into the home directory and records each copy: the managed files, the zsh alias block, the tracked Claude settings, and the Engram and agent-browser registrations.
package install

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/checkout"
)

const (
	// Fully qualified: the bare engram is ambiguous in that tap, and `brew uninstall engram` resolves it to a cask and refuses.
	engramFormula       = "gentleman-programming/tap/engram"
	agentBrowserFormula = "agent-browser"
)

const usage = `Usage: harness install [--skip-packages] [--no-backup]
       harness install --uninstall [--client <name>]... [--component <name>]... [--yes]
       harness install --clear-backups

  --skip-packages
                 Reconcile the policy, command, skill, and role agent copies,
                 the tracked client settings, and the Engram and agent-browser
                 registrations, without installing packages or configuring
                 Worktrunk's shell integration. No package manager is called,
                 so Engram, agent-browser, and Claude Code have to be installed
                 already; a full run is what installs them.
  --no-backup    Delete whatever is in the way instead of moving it to
                 ~/.dotfiles-backups. Destructive and unrecoverable: a private
                 skill or a hand-edited config sitting at a managed path is
                 gone, not archived.
  --uninstall    Remove the harness-managed configuration recorded in
                 ~/.harness/state.json: the copies it wrote, the tracked keys
                 in the Claude config, the ~/.zshrc alias block, and the Engram
                 and agent-browser registrations. A copy edited since it was
                 written stays. Binaries, packages, ~/.engram, ~/.agent-browser,
                 and ~/.dotfiles-backups are not touched. It asks before
                 removing anything.
  --client       With --uninstall, limit it to claude or opencode; repeatable.
  --component    With --uninstall, limit it to agents, aliases, commands, mcp,
                 policy, settings, or skills; repeatable. A copy another
                 installed client still uses stays.
  --yes          With --uninstall, skip the confirmation.
  --clear-backups
                 Delete every archive under ~/.dotfiles-backups/harness/,
                 which is where an install moves whatever was in the way of a
                 managed path. Destructive and unrecoverable, and it is the
                 only copy of anything of yours an install displaced. Touches
                 nothing else in ~/.dotfiles-backups and installs nothing;
                 runs on its own.

Install and uninstall run from any directory. An install copies this binary
to ~/.local/bin/harness, where the Claude Code status line runs it, copies the
assets it was built with into each client, and records every copy in
~/.harness/state.json. Set HARNESS_ASSETS to a directory to install that tree
instead.
`

const summaryText = `Next steps (interactive; not run by this installer):
  1. Authenticate: claude auth login
  2. Authenticate OpenCode natively: /connect inside OpenCode, then OpenAI, then
     ChatGPT Plus/Pro. "Manually enter API Key" bills the API instead.
  3. Authenticate: gh auth login
  4. Restart Claude Code and OpenCode.
  5. Invoke setup-harness in each project that should use the public lifecycle.

Status line: harness statusline, which Claude Code runs on every draw.
OpenCode exposes no status line surface, so it gets none.

Commands use /name in Claude Code and OpenCode.
Yolo aliases (ccyolo, ocyolo) are sourced from ~/.harness/aliases.sh; open a
new shell or run: source ~/.zshrc

Standalone third-party skills were not downloaded or updated. Their committed
bodies and skills-lock.json are the reviewed source of truth.
`

// fatal is what die panics with, and only recoverFatal recovers it, so any step can stop the run with one message instead of threading an error through each call.
type fatal string

func die(format string, args ...any) {
	panic(fatal(fmt.Sprintf(format, args...)))
}

func must(err error) {
	if err != nil {
		die("%v", err)
	}
}

type installer struct {
	home, config                      string
	dir                               checkout.Checkout
	skipPackages, noBackup, uninstall bool
	yes                               bool
	clients, components               []string
	installed                         []entry
	backupRoot                        string
	stdout, stderr                    io.Writer
}

func (in *installer) log(format string, args ...any) {
	fmt.Fprintf(in.stdout, "\033[1;34m==>\033[0m "+format+"\n", args...)
}

func (in *installer) warn(format string, args ...any) {
	fmt.Fprintf(in.stderr, "\033[1;33mharness install:\033[0m "+format+"\n", args...)
}

// recoverFatal turns a die into one red error line and exit 1. It works only when deferred directly.
func recoverFatal(stderr io.Writer, code *int) {
	if r := recover(); r != nil {
		msg, ok := r.(fatal)
		if !ok {
			panic(r)
		}
		fmt.Fprintf(stderr, "\033[1;31mharness install:\033[0m %s\n", msg)
		*code = 1
	}
}

// resolveLocations reads the two roots every managed path hangs from and refuses a relative one before anything writes: a relative HOME turns each destination into a write inside the invocation directory. XDG_CONFIG_HOME falls back to $HOME/.config, so an absolute HOME makes it absolute too.
func (in *installer) resolveLocations() {
	in.home = os.Getenv("HOME")
	in.config = os.Getenv("XDG_CONFIG_HOME")
	if in.config == "" {
		in.config = filepath.Join(in.home, ".config")
	}
	if !filepath.IsAbs(in.home) || !filepath.IsAbs(in.config) {
		die("HOME and XDG_CONFIG_HOME must be absolute paths; got HOME=%q and XDG_CONFIG_HOME=%q.", in.home, in.config)
	}
}

// Command runs `harness install` with its arguments and returns the exit code.
func Command(args []string, stdout, stderr io.Writer) (code int) {
	defer recoverFatal(stderr, &code)
	in := &installer{stdout: stdout, stderr: stderr}
	clearBackups := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--skip-packages":
			in.skipPackages = true
		case "--no-backup":
			in.noBackup = true
		case "--uninstall":
			in.uninstall = true
		case "--clear-backups":
			clearBackups = true
		case "--yes":
			in.yes = true
		case "--client", "--component":
			valid := allClients
			list := &in.clients
			if arg == "--component" {
				valid, list = allComponents, &in.components
			}
			if i+1 == len(args) || !slices.Contains(valid, args[i+1]) {
				fmt.Fprintf(stderr, "harness install: %s takes one of %s; run harness install --help\n", arg, strings.Join(valid, ", "))
				return 2
			}
			i++
			if !slices.Contains(*list, args[i]) {
				*list = append(*list, args[i])
			}
		case "-h", "--help":
			fmt.Fprint(stdout, usage)
			return 0
		default:
			fmt.Fprintf(stderr, "harness install: unknown argument %q; run harness install --help\n", arg)
			return 2
		}
	}
	// Contradictory rather than redundant: --skip-packages names a subset of the install to write, and --uninstall writes none of it. An uninstall deletes rather than archives, so --no-backup beside it is redundant and allowed.
	if in.uninstall && in.skipPackages {
		fmt.Fprintln(stderr, "harness install: --uninstall and --skip-packages are contradictory; --uninstall installs nothing.")
		return 2
	}
	if clearBackups && (in.uninstall || in.skipPackages) {
		fmt.Fprintln(stderr, "harness install: --clear-backups runs on its own; it neither installs nor uninstalls.")
		return 2
	}
	if !in.uninstall && (in.yes || in.clients != nil || in.components != nil) {
		fmt.Fprintln(stderr, "harness install: --client, --component, and --yes apply only to --uninstall.")
		return 2
	}
	in.noBackup = in.noBackup || in.uninstall
	in.resolveLocations()
	if clearBackups {
		// It reads only $HOME, so it runs from anywhere.
		in.clearBackups()
		return 0
	}
	defer in.extractAssets()()
	if in.uninstall {
		in.uninstallSelected()
	} else {
		in.install()
	}
	return 0
}

func (in *installer) install() {
	// First, so the warning precedes the two steps it is about.
	in.warnUnlessZsh()
	in.installBinary()
	if !in.skipPackages {
		in.installPackages()
		in.installIntegrations()
	}
	in.installFiles()
	in.reconcileAliases(false)
	// A package-skipping run skips the package work, so it reconciles Engram unconditionally: setup rewrites the absolute binary path each client pins, which a presence probe never checked.
	if in.skipPackages {
		in.installEngram()
	}
	// Both after Engram, whose setup rewrites settings.json and opencode.json from its own template: the harness is the last writer, which is what makes the tracked values the ones a client reads.
	in.installAgentBrowser()
	in.reconcileClaudeSettings()
	in.checkInstall()
	if !in.skipPackages {
		in.checkTools()
	}
	in.summary()
}

// run executes a command on the installer's own output and dies when it fails.
func (in *installer) run(stdin io.Reader, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, in.stdout, in.stderr
	if err := cmd.Run(); err != nil {
		die("%s %s failed: %v", name, strings.Join(args, " "), err)
	}
}

// installPackages installs every tool through Homebrew on both platforms, because three of them (opencode, engram, worktrunk) ship only as taps. It inventories the installed formulae once and installs only what is missing: an already-installed dependency keeps whatever version it has, never upgraded behind the user's back.
func (in *installer) installPackages() {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		die("The harness installer supports macOS and Linux, not %s.", runtime.GOOS)
	}
	if _, err := exec.LookPath("brew"); err != nil {
		die("Homebrew is required; run the root ./macos.sh or ./linux.sh first.")
	}
	// ~/.local/bin joins PATH first, because that is where this installer and a prior run's Claude Code put their binaries, and the presence check below has to see them.
	must(os.Setenv("PATH", filepath.Join(in.home, ".local", "bin")+string(os.PathListSeparator)+os.Getenv("PATH")))
	in.log("Installing missing AI clients and harness tools")
	in.run(nil, "brew", "tap", "anomalyco/tap")
	in.run(nil, "brew", "tap", "gentleman-programming/tap")
	installed := in.installedFormulae()
	for _, formula := range []string{"node", "gh", "uv", "worktrunk", agentBrowserFormula, "anomalyco/tap/opencode", engramFormula} {
		if !installed[shortName(formula)] {
			in.run(nil, "brew", "install", formula)
		}
	}
	// The formula's caveat, run rather than printed: without the browser every call through the MCP fails mid-session. Linux needs its system libraries too.
	in.log("Downloading Chrome for Testing for agent-browser")
	if runtime.GOOS == "darwin" {
		in.run(nil, "agent-browser", "install")
	} else {
		in.run(nil, "agent-browser", "install", "--with-deps")
	}
	// Only when absent: Anthropic's installer would put a newer channel over whatever the user already runs, the same implicit upgrade the formulae no longer get.
	if _, err := exec.LookPath("claude"); err != nil {
		in.log("Installing Claude Code from Anthropic's latest native channel")
		in.run(nil, "bash", "-c", "set -o pipefail; curl -fsSL https://claude.ai/install.sh | bash")
	}
}

// installedFormulae inventories Homebrew once, so a full install decides what to add without probing per formula. It returns the short names `brew list` reports, which is what a tap formula is known by there.
func (in *installer) installedFormulae() map[string]bool {
	cmd := exec.Command("brew", "list", "--formula")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// Without a readable inventory the run cannot tell missing from present, so it stops with brew's own diagnostic rather than reinstalling everything, which would upgrade what the contract says to leave alone.
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = "no diagnostic"
		}
		die("Could not inventory the installed Homebrew formulae: %v: %s", err, detail)
	}
	installed := map[string]bool{}
	for _, name := range strings.Fields(string(out)) {
		// Both forms: brew prints the bare name unless an ambiguity makes it qualify the tap.
		installed[name] = true
		installed[shortName(name)] = true
	}
	return installed
}

// shortName is the name `brew list` reports a formula by, dropping a tap prefix the list never carries.
func shortName(formula string) string {
	return formula[strings.LastIndex(formula, "/")+1:]
}

func (in *installer) installIntegrations() {
	in.installEngram()
	in.log("Reconciling Worktrunk")
	in.run(nil, "wt", "config", "shell", "install", "--yes", "zsh")
}

// warnUnlessZsh tells a user on another shell what they did not get: the aliases go into ~/.zshrc and Worktrunk gets its zsh integration.
func (in *installer) warnUnlessZsh() {
	shell := os.Getenv("SHELL")
	if shell != "" && filepath.Base(shell) == "zsh" {
		return
	}
	if shell == "" {
		shell = "unknown"
	}
	in.warn("Your login shell is %s, not zsh. The yolo aliases go into\n~/.zshrc and Worktrunk gets its zsh integration. Source\n%s from your own shell rc, and run\n`wt config shell install` for the shell you use.", shell, in.dir.Aliases())
}

var description = regexp.MustCompile(`(?m)^description:`)

// checkInstall verifies what every run installs, so the mode that exists for reconciling confirms each copy landed.
func (in *installer) checkInstall() {
	for _, skill := range entries(in.dir.Skills()) {
		if !isFile(filepath.Join(skill, "SKILL.md")) {
			die("Missing SKILL.md: %s", skill)
		}
		if exists(filepath.Join(in.config, "opencode", "skills", filepath.Base(skill))) {
			die("Duplicate OpenCode skill path: %s", skill)
		}
	}
	if !isFile(filepath.Join(in.dir.Skills(), "ponytail", "SKILL.md")) {
		die("Missing the ponytail skill body.")
	}
	in.checkFiles()
	if !in.trackedSettingsHold(false) {
		die("Tracked Claude settings are not reconciled in %s.", in.settingsPath())
	}
	in.checkStatusline()
}

// checkStatusline probes the binary the tracked statusLine runs on every draw, in every mode, because every mode writes that setting. A missing file, a lost mode bit, or a binary predating the subcommand all fail here rather than as a silently empty status line.
func (in *installer) checkStatusline() {
	binary := filepath.Join(in.home, ".local", "bin", "harness")
	if info, err := os.Stat(binary); err != nil || info.Mode().Perm()&0o111 == 0 {
		die("The status line binary is missing or not executable: %s; run harness install.", binary)
	}
	if exec.Command(binary, "statusline", "--help").Run() != nil {
		die("The status line binary has no statusline subcommand: %s; run harness install.", binary)
	}
}

// checkTools verifies what only a full run installs, since each assertion needs a client or a package on PATH.
func (in *installer) checkTools() {
	for _, name := range []string{"claude", "opencode", "gh", "engram", "wt", "agent-browser"} {
		if _, err := exec.LookPath(name); err != nil {
			die("Missing required command: %s", name)
		}
	}
	in.assertPluginCount("engram", 1)
	// The scope too: a leftover project-scoped entry would satisfy a bare get while the user scope stayed empty. Engram reaches Claude Code as a plugin plus this file rather than an mcp entry.
	get := exec.Command("claude", "mcp", "get", "agent-browser")
	get.Stderr = in.stderr
	if scope, err := get.Output(); err != nil || !strings.Contains(string(scope), "Scope: User config") {
		die("agent-browser is not registered in the Claude Code user scope.")
	}
	if engram := filepath.Join(in.home, ".claude", "mcp", "engram.json"); !isFile(engram) {
		die("Engram's Claude Code MCP file is missing: %s", engram)
	}
	// Through a file: `opencode debug config` stops at the 64 KiB pipe capacity whenever stdout is not a regular file, and this config is past it.
	resolved, err := os.CreateTemp("", "harness-opencode-*")
	must(err)
	defer os.Remove(resolved.Name())
	debug := exec.Command("opencode", "debug", "config")
	debug.Stdout, debug.Stderr = resolved, in.stderr
	if err := debug.Run(); err != nil {
		die("opencode debug config failed: %v", err)
	}
	must(resolved.Close())
	if !in.opencodeRegistered(resolved.Name()) {
		die("OpenCode is missing an expected plugin or MCP server.")
	}
	validate := exec.Command("claude", "plugin", "validate", in.dir.Skills())
	validate.Stderr = in.stderr
	if err := validate.Run(); err != nil {
		die("claude plugin validate failed: %v", err)
	}
	if help, _ := exec.Command("wt", "switch", "--help").Output(); !strings.Contains(string(help), "--no-cd") {
		die("Worktrunk's wt switch has no --no-cd flag; upgrade Worktrunk.")
	}
	// state and dash pass --config-set to `wt list`, so an older Worktrunk that lacks it fails here rather than inside a later command. The flag is probed where it is used, because a global option may be hidden from the subcommand that needs it.
	help, err := exec.Command("wt", "list", "--help").Output()
	if err != nil {
		die("Could not read Worktrunk's wt list help: %v", err)
	}
	if !strings.Contains(string(help), "--config-set") {
		die("Worktrunk's wt list has no --config-set flag; upgrade Worktrunk to 0.61 or newer.")
	}
}

func (in *installer) assertPluginCount(plugin string, expected int) {
	out, err := exec.Command("claude", "plugin", "list", "--json").Output()
	var doc any
	if err != nil || json.Unmarshal(out, &doc) != nil {
		die("Could not list the Claude Code plugins.")
	}
	items, _ := doc.([]any)
	if m, ok := doc.(map[string]any); ok {
		items, _ = m["installed"].([]any)
	}
	count := 0
	for _, item := range items {
		m, _ := item.(map[string]any)
		id, _ := m["pluginId"].(string)
		if id == "" {
			id, _ = m["id"].(string)
		}
		if strings.Contains(id, plugin) {
			count++
		}
	}
	if count != expected {
		die("Expected %d %s integration in Claude Code; found %d.", expected, plugin, count)
	}
}

// opencodeRegistered checks the resolved config for what OpenCode loads, and the global file for proof this installer put agent-browser there, since a project-root opencode.json would satisfy the resolved view on its own.
func (in *installer) opencodeRegistered(resolvedPath string) bool {
	binary, _ := exec.LookPath("agent-browser")
	command := []any{binary, "mcp"}
	resolved, ok := decodeFile(resolvedPath)
	if !ok {
		return false
	}
	plugins, _ := resolved["plugin"].([]any)
	engrams := 0
	for _, plugin := range plugins {
		if name, ok := plugin.(string); ok && strings.Contains(name, "engram") {
			engrams++
		}
	}
	servers, _ := resolved["mcp"].(map[string]any)
	server, _ := servers["agent-browser"].(map[string]any)
	if _, ok := servers["engram"]; engrams != 1 || !ok || server == nil ||
		server["type"] != "local" || server["enabled"] != true || !reflect.DeepEqual(server["command"], command) {
		return false
	}
	global, ok := decodeFile(filepath.Join(in.config, "opencode", "opencode.json"))
	globalServers, _ := global["mcp"].(map[string]any)
	globalServer, _ := globalServers["agent-browser"].(map[string]any)
	return ok && reflect.DeepEqual(globalServer["command"], command)
}

func (in *installer) summary() {
	fmt.Fprint(in.stdout, "\n\033[1;32m✓ Harness AI installed.\033[0m\n\n"+summaryText)
	if in.backupRoot != "" {
		fmt.Fprintf(in.stdout, "\nBackups: %s\n", in.backupRoot)
	}
}

// size reports a path's disk usage the way `du -sh` prints it, or "unknown".
func size(path string) string {
	out, _ := exec.Command("du", "-sh", path).Output()
	if fields := strings.Fields(string(out)); len(fields) > 0 {
		return fields[0]
	}
	return "unknown"
}

// clearBackups deletes every archive an install left under ~/.dotfiles-backups/harness and nothing beside it. Each root goes separately so the log names what was deleted, the only record left afterwards.
func (in *installer) clearBackups() {
	root := filepath.Join(in.home, ".dotfiles-backups", "harness")
	if !isDir(root) {
		in.log("No harness backups to delete at %s", root)
		return
	}
	reclaimed := size(root)
	count := 0
	for _, path := range entries(root) {
		must(os.RemoveAll(path))
		in.log("Deleted %s", path)
		count++
	}
	// Only when empty, and never the parent: refusing a non-empty directory is the guard.
	_ = os.Remove(root)
	in.log("Deleted %d harness backup(s), %s reclaimed.", count, reclaimed)
}

// installBinary copies the running binary to ~/.local/bin/harness, where the status line runs it, unless it already is that file.
func (in *installer) installBinary() {
	bin := filepath.Join(in.home, ".local", "bin", "harness")
	self, err := os.Executable()
	must(err)
	if self, err = filepath.EvalSymlinks(self); err != nil {
		die("Could not resolve the running binary: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil && (resolved == self || digest(resolved) == digest(self)) {
		return
	}
	src, err := os.Open(self)
	must(err)
	defer src.Close()
	must(os.MkdirAll(filepath.Dir(bin), 0o755))
	tmp := bin + ".next"
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	must(err)
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		die("Copying harness to %s failed: %v", bin, err)
	}
	must(dst.Close())
	// A rename, so a status line drawing mid-install runs the old binary or the new one, never a partial file.
	must(os.Rename(tmp, bin))
	in.log("Installed %s", bin)
}
