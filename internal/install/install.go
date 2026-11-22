package install

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/agents"
	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/models"
	"github.com/jvillegasd/harness/internal/skills"
	"github.com/jvillegasd/harness/internal/ui"
)

const (
	engramFormula       = "gentleman-programming/tap/engram"
	agentBrowserFormula = "agent-browser"
)

const usage = `Usage: harness install [<component>...] [--client <name>]... [--no-backup] [--verbose]

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
            shell's startup file (~/.zshrc for zsh, ~/.bashrc for bash on
            Linux, the bash login profile on macOS)

  --client     Limit the run to claude, codex, or opencode; repeatable.
               Without it, the run configures every client that is installed
               or that this run installs.
  --no-backup  Delete files at managed paths instead of archiving them in
               ~/.dotfiles-backups. Private skills and hand-edited configs at
               those paths will be lost.
  --verbose    List every file the run writes, removes, or keeps, instead of
               a count per component.

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

const uninstallUsage = `Usage: harness uninstall [<component>...] [--client <name>]... [--all] [--yes] [--verbose]
       harness uninstall --clear-backups

Remove the named components. With none named, remove the managed config of
every component (skills, agents, floor, settings, aliases, and the MCP
registrations) and leave every tool installed.

  --client     Limit it to claude, codex, or opencode; repeatable. A copy or
               tool another client still uses stays.
  --all        Remove every component, the tools included, and then
               ~/.local/bin/harness. Not combined with component names.
  --yes        Skip the confirmation.
  --verbose    List every file removed or kept, instead of a count per
               component.
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

// nextSteps are what the human runs after a full install.
var nextSteps = []string{
	"Authenticate: claude auth login",
	"Authenticate Codex: codex login",
	"Authenticate OpenCode natively: /connect inside OpenCode, then OpenAI, then ChatGPT Plus/Pro. \"Manually enter API Key\" bills the API instead.",
	"Authenticate: gh auth login",
	"Restart Claude Code, Codex, and OpenCode.",
	"In Codex, open /hooks and trust the harness compaction hook in ~/.codex/hooks.json; Codex skips a new or changed hook until then.",
	"Invoke setup-harness in each project that should use a tracker or critical paths.",
}

// closingNotes follow the next steps after a full install.
var closingNotes = []string{
	"Status line: harness statusline, which Claude Code runs on every draw. Codex and OpenCode get none from the harness.",
	"Workflow skills also answer to /name in Claude Code, to @name in OpenCode v2, and to $name in Codex.",
	"Yolo aliases (ccyolo, cxyolo, ocyolo) are sourced from ~/.harness/aliases.sh; open a new shell to load them.",
	"Standalone third-party skills were not downloaded or updated. Their committed bodies and skills-lock.json are the reviewed source of truth.",
}

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
	verbose             bool
	book                logbook
	clients, components []string
	installed           []entry
	aliasesStripped     []string
	backupRoot          string
	assignments         map[string]agents.Assigned
	stdout, stderr      io.Writer
	// keepEdited leaves a managed copy changed since it was recorded in place,
	// with its record, instead of archiving and replacing it.
	keepEdited bool
	keptEdited map[string]bool
}

func recoverFatal(verb string, stderr io.Writer, code *int) {
	if r := recover(); r != nil {
		msg, ok := r.(fatal)
		if !ok {
			panic(r)
		}
		fmt.Fprintln(stderr, ui.For(stderr).Fail(fmt.Sprintf("harness %s: %s", verb, msg)))
		*code = 1
	}
}

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

func inOrder(list, order []string) []string {
	var out []string
	for _, item := range order {
		if slices.Contains(list, item) {
			out = append(out, item)
		}
	}
	return out
}

func Command(args []string, stdout, stderr io.Writer) (code int) {
	defer recoverFatal("install", stderr, &code)
	in := &installer{verb: "install", stdout: stdout, stderr: stderr}
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--no-backup":
			in.noBackup = true
		case "--verbose":
			in.verbose = true
		case "-h", "--help":
			fmt.Fprint(stdout, usage)
			return 0
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
		case "--verbose":
			in.verbose = true
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

	in.noBackup = true
	in.resolveLocations()
	fmt.Fprintln(stdout, ui.For(stdout).Title("harness uninstall"))
	if clearBackups {

		in.clearBackups()
		return 0
	}
	defer in.extractAssets()()
	in.uninstallSelected()
	return 0
}

func (in *installer) has(component string) bool { return slices.Contains(in.components, component) }

func (in *installer) targets(client string) bool { return slices.Contains(in.clients, client) }

var clientNames = map[string]string{"claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode"}

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
	defer in.flushOnPanic()

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

	alsoSettings := in.has("mcps") && !in.has("settings")
	if alsoSettings {
		in.components = append(in.components, "settings")
	}
	in.components = inOrder(in.components, allComponents)
	in.usePATH()
	in.resolveTargets()
	scope := "all components"
	if len(in.components) < len(allComponents) {
		scope = strings.Join(in.components, ", ")
	}
	fmt.Fprintln(in.stdout, in.style().Title("harness install", strings.Join(in.clients, ", "), scope))
	if alsoSettings {
		in.log("mcps: also reconciling settings, which engram setup rewrites")
	}

	if in.targets("opencode") && in.has("settings") {
		in.openCodeConfig()
	}
	in.begin("tools", "this binary and the tools the components need")
	in.installBinary()
	in.installTools()
	in.end("ready")
	if slices.ContainsFunc(in.components, func(c string) bool { return c != "mcps" && slices.Contains(configComponents, c) }) {
		in.book.deferring = true
		in.installFiles()
		in.book.deferring = false
	}
	if in.has("skills") {
		in.begin("skills", "for "+strings.Join(in.clients, ", "))
		in.flush("skills")
		in.end(in.counted("skills"))
	}
	if in.has("agents") {
		in.begin("agents", fmt.Sprintf("%d roles for %s", len(models.AllRoles), strings.Join(in.clients, ", ")))
		in.drawAssignments()
		in.flush("agents")
		in.end(in.counted("agents"))
	}
	var config []string
	for _, c := range []string{"floor", "settings", "aliases", "mcps"} {
		if in.has(c) {
			config = append(config, c)
		}
	}
	if len(config) > 0 || slices.ContainsFunc(in.book.deferred, func(line heldLine) bool {
		return slices.Contains([]string{"floor", "settings", "aliases", ""}, line.component)
	}) {
		in.begin("config", strings.Join(config, ", "))
		for _, c := range []string{"floor", "settings", "aliases"} {
			if in.has(c) && in.book.files[c] != nil {
				in.log("%s  %s", ui.Pad(c, 8), in.style().Dim(in.counted(c)))
			}
		}
		in.flush("floor", "settings", "aliases", "")
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
	in.end("reconciled")
	in.checkInstall()
	in.summary()
}

func (in *installer) run(stdin io.Reader, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, in.stdout, in.stderr
	if err := cmd.Run(); err != nil {
		die("%s %s failed: %v", name, strings.Join(args, " "), err)
	}
}

var description = regexp.MustCompile(`(?m)^description:`)

func (in *installer) checkInstall() {
	if in.has("skills") {
		mode := false
		for _, skill := range in.skills() {
			if !isFile(filepath.Join(skill.Dir, "SKILL.md")) {
				die("Missing SKILL.md: %s", skill.Dir)
			}
			if exists(filepath.Join(in.config, "opencode", "skills", skill.Name)) {
				die("Duplicate OpenCode skill path: %s", skill.Dir)
			}
			mode = mode || skill.Name == "jvillegasd-mode"
		}
		if !mode {
			die("Missing the jvillegasd-mode skill body.")
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

func (in *installer) checkStatusline() {
	binary := filepath.Join(in.home, ".local", "bin", "harness")
	if info, err := os.Stat(binary); err != nil || info.Mode().Perm()&0o111 == 0 {
		die("The status line binary is missing or not executable: %s; run harness install.", binary)
	}
	if exec.Command(binary, "statusline", "--help").Run() != nil {
		die("The status line binary has no statusline subcommand: %s; run harness install.", binary)
	}
}

func (in *installer) checkTools() {
	for _, t := range in.selectedTools() {
		if !present(t.name) {
			die("Missing required command: %s", t.name)
		}
	}
	if in.has("mcps") && in.targets("claude") {
		in.assertPluginCount("engram", 1)

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

func (in *installer) opencodeRegistered(resolvedPath string) bool {
	binary, _ := exec.LookPath("agent-browser")
	command := []any{binary, "mcp"}
	plugins, servers, ok := in.opencodeLoaded(resolvedPath)
	if !ok {
		return false
	}
	engrams := 0
	for _, plugin := range plugins {
		if name, ok := plugin.(string); ok && strings.Contains(name, "engram") {
			engrams++
		}
	}
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

func (in *installer) opencodeLoaded(path string) (plugins []any, servers map[string]any, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false
	}
	var merged map[string]any
	if json.Unmarshal(data, &merged) == nil {
		plugins, _ = merged["plugin"].([]any)
		servers, _ = merged["mcp"].(map[string]any)
		return plugins, servers, true
	}
	var documents []struct {
		Info map[string]any `json:"info"`
	}
	if json.Unmarshal(data, &documents) != nil {
		return nil, nil, false
	}
	servers = map[string]any{}
	for _, document := range documents {
		listed, _ := document.Info["plugin"].([]any)
		plugins = append(plugins, listed...)
		mcp, _ := document.Info["mcp"].(map[string]any)
		declared, _ := mcp["servers"].(map[string]any)
		for name, value := range declared {
			server, ok := value.(map[string]any)
			if !ok {
				continue
			}
			merged, _ := servers[name].(map[string]any)
			if merged == nil {
				merged = map[string]any{}
			}
			maps.Copy(merged, server)
			servers[name] = merged
		}
	}
	for _, value := range servers {
		server := value.(map[string]any)
		server["enabled"] = server["disabled"] != true && server["enabled"] != false
	}
	files, _ := os.ReadDir(filepath.Join(in.config, "opencode", "plugins"))
	for _, file := range files {
		plugins = append(plugins, file.Name())
	}
	return plugins, servers, true
}

func (in *installer) summary() {
	if len(in.components) < len(allComponents) {
		in.summarize(fmt.Sprintf("Harness AI installed: %s for %s.", strings.Join(in.components, ", "), strings.Join(in.clients, ", ")))
	} else {
		in.summarize("Harness AI installed.")
		in.drawList("Next steps "+in.style().Dim("interactive; not run by this installer"), numbered(nextSteps))
		fmt.Fprintln(in.stdout)
		for _, note := range closingNotes {
			fmt.Fprintln(in.stdout, in.style().Dim(note))
		}
	}
	if in.backupRoot != "" {
		fmt.Fprintln(in.stdout, "\n"+in.style().Dim("Backups")+"  "+in.tilde(in.backupRoot))
	}
}

func size(path string) string {
	out, _ := exec.Command("du", "-sh", path).Output()
	if fields := strings.Fields(string(out)); len(fields) > 0 {
		return fields[0]
	}
	return "unknown"
}

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

	_ = os.Remove(root)
	in.log("Deleted %d harness backup(s), %s reclaimed.", count, reclaimed)
}

func (in *installer) binaryPath() string { return filepath.Join(in.home, ".local", "bin", "harness") }

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

	must(os.Rename(tmp, bin))
	in.log("Installed %s", bin)
}
