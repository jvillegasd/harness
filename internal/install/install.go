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
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/agents"
	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/skills"
)

const (
	// Fully qualified: the bare engram is ambiguous in that tap, and `brew uninstall engram` resolves it to a cask and refuses.
	engramFormula       = "gentleman-programming/tap/engram"
	agentBrowserFormula = "agent-browser"
)

const usage = `Usage: harness install [<component>...] [--client <name>]... [--no-backup]

Install the named components, or every component when none is named. Each
choice applies to this run only.

Components:
  clients   Claude Code, Codex, and OpenCode, when missing
  packages  node, gh, and uv, when missing
  mcps      Engram and agent-browser when missing, and their registration in
            each client; also reconciles settings, which Engram's setup rewrites
  skills    The skills, copied into each client
  agents    The role agents, rendered for each client
  floor     The floor, as each client's global instructions
  settings  Tracked client settings, compaction hooks, and command rules
  aliases   The yolo aliases and the line that loads them from your login
            shell's startup file (~/.zshrc, or ~/.bashrc for bash)

  --client     Limit the run to claude, codex, or opencode; repeatable.
               Without it, the run configures every client that is installed
               or that this run installs.
  --no-backup  Delete files at managed paths instead of archiving them in
               ~/.dotfiles-backups. Private skills and hand-edited configs at
               those paths will be lost.

A tool counts as installed when it is on PATH, however it got there, and is
never reinstalled or upgraded; Homebrew and the network are used only for a
missing one. The install records each tool it adds, which is what
harness uninstall removes. It also copies this binary to ~/.local/bin/harness,
where the Claude Code status line runs it, and records every managed copy in
~/.harness/state.json. Codex is always configured in ~/.codex, whatever
CODEX_HOME is set to. Set HARNESS_ASSETS to a directory to install that tree
instead; it must carry skills-lock.json, which tells an install which skills
are locally authored and so checked for invocation guards. Install runs from
any directory.

Details: docs/cli.md#install-and-uninstall
`

const uninstallUsage = `Usage: harness uninstall [<component>...] [--client <name>]... [--all] [--yes]
       harness uninstall --clear-backups

Remove the named components. With none named, remove the managed config of
every component (skills, agents, floor, settings, aliases, and the MCP
registrations) and leave every tool installed.

  --client     Limit it to claude, codex, or opencode; repeatable. A copy or
               tool another client still uses stays.
  --all        Remove every component, the tools included, and then
               ~/.local/bin/harness. Not combined with component names.
  --yes        Skip the confirmation.
  --clear-backups
               Delete every archive under ~/.dotfiles-backups/harness/.
               These archives hold files displaced from managed paths.
               Nothing else in ~/.dotfiles-backups is touched. Runs alone.

Naming clients, packages, or mcps removes the tools the harness recorded
installing, the way it installed them. A tool that was on the machine before
the harness stays. No uninstall deletes a tool's data (~/.engram,
~/.agent-browser, ~/.claude, ~/.codex) or ~/.dotfiles-backups. Edited copies
stay. ~/.harness, with the harness models and review level choices, goes only
when nothing recorded remains. Prompts before removing anything.

Details: docs/cli.md#install-and-uninstall
`

const summaryText = `Next steps (interactive; not run by this installer):
  1. Authenticate: claude auth login
  2. Authenticate Codex: codex login
  3. Authenticate OpenCode natively: /connect inside OpenCode, then OpenAI, then
     ChatGPT Plus/Pro. "Manually enter API Key" bills the API instead.
  4. Authenticate: gh auth login
  5. Restart Claude Code, Codex, and OpenCode.
  6. In Codex, open /hooks and trust the harness compaction hook in
     ~/.codex/hooks.json; Codex skips a new or changed hook until then.
  7. Invoke setup-harness in each project that should use the public lifecycle.

Status line: harness statusline, which Claude Code runs on every draw.
Codex and OpenCode get none from the harness.

Workflow skills also answer to /name in Claude Code and OpenCode, and to
$name in Codex.
Yolo aliases (ccyolo, cxyolo, ocyolo) are sourced from ~/.harness/aliases.sh;
open a new shell to load them.

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
	home, config        string
	dir                 checkout.Checkout
	verb                string
	noBackup, yes, all  bool
	clients, components []string
	installed           []entry
	aliasesStripped     []string
	backupRoot          string
	assignments         map[string]agents.Assigned
	stdout, stderr      io.Writer
}

func (in *installer) log(format string, args ...any) {
	fmt.Fprintf(in.stdout, "\033[1;34m==>\033[0m "+format+"\n", args...)
}

func (in *installer) warn(format string, args ...any) {
	fmt.Fprintf(in.stderr, "\033[1;33mharness %s:\033[0m "+format+"\n", append([]any{in.verb}, args...)...)
}

// recoverFatal turns a die into one red error line and exit 1. It works only when deferred directly.
func recoverFatal(verb string, stderr io.Writer, code *int) {
	if r := recover(); r != nil {
		msg, ok := r.(fatal)
		if !ok {
			panic(r)
		}
		fmt.Fprintf(stderr, "\033[1;31mharness %s:\033[0m %s\n", verb, msg)
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

// selectArg reads one selection argument at args[i], a component name or --client and its value, and returns the index of the last argument it consumed, or false after printing the diagnostic for an invalid one.
func (in *installer) selectArg(args []string, i int, stderr io.Writer) (int, bool) {
	arg := args[i]
	switch {
	case arg == "--client":
		if i+1 == len(args) || !slices.Contains(allClients, args[i+1]) {
			fmt.Fprintf(stderr, "harness %s: --client takes one of %s; run harness %s --help\n", in.verb, strings.Join(allClients, ", "), in.verb)
			return i, false
		}
		if !slices.Contains(in.clients, args[i+1]) {
			in.clients = append(in.clients, args[i+1])
		}
		return i + 1, true
	case arg == "--component":
		fmt.Fprintf(stderr, "harness %s: components are named without a flag now, as in harness %s skills agents\n", in.verb, in.verb)
		return i, false
	case slices.Contains(allComponents, arg):
		if !slices.Contains(in.components, arg) {
			in.components = append(in.components, arg)
		}
		return i, true
	case strings.HasPrefix(arg, "-"):
		fmt.Fprintf(stderr, "harness %s: unknown argument %q; run harness %s --help\n", in.verb, arg, in.verb)
	default:
		fmt.Fprintf(stderr, "harness %s: unknown component %q; it takes %s\n", in.verb, arg, strings.Join(allComponents, ", "))
	}
	return i, false
}

// inOrder sorts a selection into its canonical order, so logs and records read the same however it was typed.
func inOrder(list, order []string) []string {
	var out []string
	for _, item := range order {
		if slices.Contains(list, item) {
			out = append(out, item)
		}
	}
	return out
}

// Command runs `harness install` with its arguments and returns the exit code.
func Command(args []string, stdout, stderr io.Writer) (code int) {
	defer recoverFatal("install", stderr, &code)
	in := &installer{verb: "install", stdout: stdout, stderr: stderr}
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--no-backup":
			in.noBackup = true
		case "-h", "--help":
			fmt.Fprint(stdout, usage)
			return 0
		case "--uninstall", "--yes", "--clear-backups":
			fmt.Fprintf(stderr, "harness install: %s moved to harness uninstall; run harness uninstall --help\n", arg)
			return 2
		case "--skip-packages":
			fmt.Fprintln(stderr, "harness install: --skip-packages was removed; name the components to install instead, as in harness install skills agents")
			return 2
		default:
			next, ok := in.selectArg(args, i, stderr)
			if !ok {
				return 2
			}
			i = next
		}
	}
	in.resolveLocations()
	defer in.extractAssets()()
	in.install()
	return 0
}

// UninstallCommand runs `harness uninstall` with its arguments and returns the exit code.
func UninstallCommand(args []string, stdout, stderr io.Writer) (code int) {
	defer recoverFatal("uninstall", stderr, &code)
	in := &installer{verb: "uninstall", stdout: stdout, stderr: stderr}
	clearBackups := false
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--yes":
			in.yes = true
		case "--all":
			in.all = true
		case "--clear-backups":
			clearBackups = true
		case "-h", "--help":
			fmt.Fprint(stdout, uninstallUsage)
			return 0
		default:
			next, ok := in.selectArg(args, i, stderr)
			if !ok {
				return 2
			}
			i = next
		}
	}
	if clearBackups && len(args) > 1 {
		fmt.Fprintln(stderr, "harness uninstall: --clear-backups runs on its own; it removes no component.")
		return 2
	}
	if in.all && in.components != nil {
		fmt.Fprintln(stderr, "harness uninstall: --all already names every component; drop either --all or the names.")
		return 2
	}
	// An uninstall deletes rather than archives.
	in.noBackup = true
	in.resolveLocations()
	if clearBackups {
		// It reads only $HOME, so it runs from anywhere.
		in.clearBackups()
		return 0
	}
	defer in.extractAssets()()
	in.uninstallSelected()
	return 0
}

// has reports whether this run selected a component.
func (in *installer) has(component string) bool { return slices.Contains(in.components, component) }

// targets reports whether this run configures a client.
func (in *installer) targets(client string) bool { return slices.Contains(in.clients, client) }

// clientNames are the clients' product names, for messages.
var clientNames = map[string]string{"claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode"}

// resolveTargets settles which clients this run configures before any write: the named ones, each of which must be installed or be installed by this run; otherwise every client this run installs or that is already on PATH.
func (in *installer) resolveTargets() {
	installing := in.has("clients")
	if in.clients != nil {
		in.clients = inOrder(in.clients, allClients)
		for _, client := range in.clients {
			if !installing && !present(client) {
				die("%s is not installed; run harness install clients --client %s first.", clientNames[client], client)
			}
		}
		return
	}
	for _, client := range allClients {
		if installing || present(client) {
			in.clients = append(in.clients, client)
		}
	}
	if len(in.clients) == 0 && slices.ContainsFunc(in.components, func(c string) bool { return slices.Contains(configComponents, c) }) {
		die("No client is installed; run harness install clients first.")
	}
}

func (in *installer) install() {
	// Before any write, so a refusal leaves HOME untouched.
	unguarded, err := skills.Unguarded(in.dir)
	if err != nil {
		die("Checking the local skills' invocation guards failed: %v", err)
	}
	if len(unguarded) > 0 {
		die("Local skill missing an invocation guard: %s", strings.Join(unguarded, "; "))
	}
	if in.components == nil {
		in.components = slices.Clone(allComponents)
	}
	// Engram's setup rewrites settings.json and opencode.json from its own template, so the harness reconciles settings after it and stays the last writer.
	if in.has("mcps") && !in.has("settings") {
		in.log("mcps: also reconciling settings, which engram setup rewrites")
		in.components = append(in.components, "settings")
	}
	in.components = inOrder(in.components, allComponents)
	in.usePATH()
	in.resolveTargets()
	// A config this run could not rewrite is refused before any client is touched, so every registration, Engram's included, stays as it was.
	if in.targets("opencode") && in.has("settings") {
		in.openCodeConfig()
	}
	in.installBinary()
	in.installTools()
	if slices.ContainsFunc(in.components, func(c string) bool { return c != "mcps" && slices.Contains(configComponents, c) }) {
		in.installFiles()
	}
	if in.has("aliases") {
		in.reconcileAliases(false)
	}
	if in.has("mcps") {
		in.installEngram()
		in.installAgentBrowser()
		if in.targets("claude") {
			in.registerEngramClaudeMCP()
		}
	}
	if in.has("settings") {
		if in.targets("claude") {
			in.reconcileClaudeSettings()
		}
		if in.targets("codex") {
			in.reconcileCodexHooks(false)
		}
		if in.targets("opencode") {
			in.reconcileOpenCodePermission(false)
		}
	}
	in.updateState(func(s *state) {
		for _, client := range in.clients {
			for _, c := range in.components {
				if slices.Contains(configComponents, c) && !slices.Contains(s.Installed[client], c) {
					s.Installed[client] = append(s.Installed[client], c)
				}
			}
			if len(s.Installed[client]) > 0 {
				s.Installed[client] = sorted(s.Installed[client])
			}
		}
	})
	in.checkInstall()
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

var description = regexp.MustCompile(`(?m)^description:`)

// checkInstall verifies what this run installed, so each copy, setting, and tool it reconciled is confirmed in place.
func (in *installer) checkInstall() {
	if in.has("skills") {
		ponytail := false
		for _, skill := range in.skills() {
			if !isFile(filepath.Join(skill.Dir, "SKILL.md")) {
				die("Missing SKILL.md: %s", skill.Dir)
			}
			if exists(filepath.Join(in.config, "opencode", "skills", skill.Name)) {
				die("Duplicate OpenCode skill path: %s", skill.Dir)
			}
			ponytail = ponytail || skill.Name == "ponytail"
		}
		if !ponytail {
			die("Missing the ponytail skill body.")
		}
	}
	in.checkFiles()
	if in.has("settings") {
		if in.targets("claude") && !in.trackedSettingsHold(false) {
			die("Tracked Claude settings are not reconciled in %s.", in.settingsPath())
		}
		if in.targets("codex") && !in.codexHooksHold(false) {
			die("The harness compaction hook is not reconciled in %s.", in.codexHooksPath())
		}
		if in.targets("opencode") && !in.openCodePermissionHold(false) {
			die("The harness permission rules are not reconciled in %s.", in.openCodeConfigPath())
		}
	}
	in.checkStatusline()
	in.checkTools()
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

// checkTools verifies the tools this run selected are on PATH, and asks each targeted client whether it loads the registrations and skills this run reconciled.
func (in *installer) checkTools() {
	for _, t := range in.selectedTools() {
		if !present(t.name) {
			die("Missing required command: %s", t.name)
		}
	}
	if in.has("mcps") && in.targets("claude") {
		in.assertPluginCount("engram", 1)
		// The scope too: a leftover project-scoped entry would satisfy a bare get while the user scope stayed empty. Engram reaches Claude Code as a plugin for its hooks and skill, plus this mcp entry for its tools.
		for _, server := range []string{"agent-browser", "engram"} {
			get := exec.Command("claude", "mcp", "get", server)
			get.Stderr = in.stderr
			if scope, err := get.Output(); err != nil || !strings.Contains(string(scope), "Scope: User config") {
				die("%s is not registered in the Claude Code user scope.", server)
			}
		}
	}
	if in.has("mcps") && in.targets("codex") && in.codex("mcp", "get", "agent-browser").Run() != nil {
		die("agent-browser is not registered in Codex's %s.", in.codexHome())
	}
	if in.has("mcps") && in.targets("opencode") {
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
	}
	if in.has("skills") && in.targets("claude") {
		// The validator reads only skills/<name>/SKILL.md one level down and follows no symlink, so it gets a flat copy of the grouped tree.
		flat, err := os.MkdirTemp("", "harness-skills-*")
		must(err)
		defer os.RemoveAll(flat)
		for _, skill := range in.skills() {
			must(os.CopyFS(filepath.Join(flat, "skills", skill.Name), os.DirFS(skill.Dir)))
		}
		validate := exec.Command("claude", "plugin", "validate", filepath.Join(flat, "skills"))
		validate.Stderr = in.stderr
		if err := validate.Run(); err != nil {
			die("claude plugin validate failed: %v", err)
		}
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

// summary prints the next steps after a full install, and after a narrowed one only what it installed and where.
func (in *installer) summary() {
	if len(in.components) < len(allComponents) {
		fmt.Fprintf(in.stdout, "\n\033[1;32m✓ Harness AI installed: %s for %s.\033[0m\n", strings.Join(in.components, ", "), strings.Join(in.clients, ", "))
	} else {
		fmt.Fprint(in.stdout, "\n\033[1;32m✓ Harness AI installed.\033[0m\n\n"+summaryText)
	}
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

// binaryPath is where an install copies this binary and the status line runs it.
func (in *installer) binaryPath() string { return filepath.Join(in.home, ".local", "bin", "harness") }

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
