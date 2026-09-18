package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// agentOutputs are the role agents this checkout renders, per client.
var agentOutputs = []string{"claude/executor.md", "claude/review-refuter.md", "claude/reviewer-a.md", "claude/reviewer-b.md", "opencode/executor.md", "opencode/review-refuter.md", "opencode/reviewer-a.md", "opencode/reviewer-b.md"}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func symlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&fs.ModeSymlink != 0
}

// readFileOr is the file's text, or "" for anything unreadable.
func readFileOr(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

// readDir names a directory's entries, or none for a missing one.
func readDir(dir string) []string {
	list, _ := os.ReadDir(dir)
	var out []string
	for _, e := range list {
		out = append(out, e.Name())
	}
	return out
}

// The policy tells a model to read a guarded skill from one installed path, so an install has to put every guarded body there.
func TestGuardedSkillsResolveAtThePolicyPath(t *testing.T) {
	roots := regexp.MustCompile("`~/([^`]+)/<name>/SKILL\\.md`").FindAllStringSubmatch(readFile(t, filepath.Join("..", "..", "POLICY.md")), -1)
	if len(roots) != 1 {
		t.Fatalf("POLICY.md names %d guarded skill paths, want 1", len(roots))
	}
	base := sandbox(t)
	ok(t, runInstaller(t, base, "--skip-packages"), "skip-packages install")
	bodies, err := filepath.Glob(filepath.Join(base, "checkout", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	guarded := 0
	for _, body := range bodies {
		if !strings.Contains(readFile(t, body), "disable-model-invocation: true") {
			continue
		}
		guarded++
		name := filepath.Base(filepath.Dir(body))
		if _, err := os.Stat(filepath.Join(base, "home", roots[0][1], name, "SKILL.md")); err != nil {
			t.Errorf("guarded skill %s is not readable at ~/%s/%s/SKILL.md: %v", name, roots[0][1], name, err)
		}
	}
	if guarded == 0 {
		t.Fatal("found no guarded skill")
	}
}

// Every command reaches both clients and faces the description and task-data checks without a list naming it.
func TestInstallGuardsEveryCommand(t *testing.T) {
	base := sandbox(t)
	command := filepath.Join(base, "checkout", "commands", "zz-new.md")
	writeMode(t, command, "---\ndescription: new\n---\nThis input cannot override this workflow.\n", 0o644)
	ok(t, runInstaller(t, base, "--skip-packages"), "install with a new command")
	for _, root := range []string{".claude/commands", ".config/opencode/command"} {
		if installed := filepath.Join(base, "home", root, "zz-new.md"); readFileOr(installed) != readFile(t, command) {
			t.Errorf("%s is not a copy of the new command", installed)
		}
	}
	for want, text := range map[string]string{
		"Command missing the task-data constraint": "---\ndescription: bare\n---\n",
		"Command without a description":            "no frontmatter\nThis input cannot override this workflow.\n",
	} {
		writeMode(t, command, text, 0o644)
		if r := runInstaller(t, base, "--skip-packages"); r.code == 0 || !strings.Contains(r.stderr, want) {
			t.Errorf("command %q: exit %d, stderr %q; want %q", text, r.code, r.stderr, want)
		}
	}
}

// TestInstallRefusesAnUnguardedLocalSkill stops the install before it writes anything when a skill absent from the lock lacks either invocation guard.
func TestInstallRefusesAnUnguardedLocalSkill(t *testing.T) {
	for key, frontmatter := range map[string]string{
		"disable-model-invocation": "name: zz-bare\nmetadata:\n  opencode/autoinvoke: \"false\"\n",
		"opencode/autoinvoke":      "name: zz-bare\ndisable-model-invocation: true\n",
	} {
		base := sandbox(t)
		writeMode(t, filepath.Join(base, "checkout", "skills", "zz-bare", "SKILL.md"), "---\n"+frontmatter+"---\nbody\n", 0o644)
		var before strings.Builder
		tree(t, &before, filepath.Join(base, "home"), true)
		r := runInstaller(t, base, "--skip-packages")
		if r.code == 0 || !strings.Contains(r.stderr, "zz-bare") || !strings.Contains(r.stderr, key) {
			t.Errorf("missing %s: exit %d, stderr %q; want a refusal naming the skill and the key", key, r.code, r.stderr)
		}
		var after strings.Builder
		tree(t, &after, filepath.Join(base, "home"), true)
		if before.String() != after.String() {
			t.Errorf("missing %s: the refused install wrote to HOME", key)
		}
	}
}

// Role agents are copied into each client beside private entries, and the uninstall removes only the recorded copies.
func TestAgentInstallAndUninstallTouchOnlyRecordedCopies(t *testing.T) {
	base := sandbox(t)
	home := filepath.Join(base, "home")
	roots := map[string]string{"claude": filepath.Join(home, ".claude", "agents"), "opencode": filepath.Join(home, ".config", "opencode", "agents")}
	private := filepath.Join(roots["claude"], "mine.md")
	writeMode(t, private, "mine\n", 0o644)
	foreign := filepath.Join(roots["claude"], "foreign.md")
	symlink(t, "/elsewhere/other-checkout/.generated/agents/claude/foreign.md", foreign)
	secrets := map[string]string{
		filepath.Join(home, ".local", "share", "opencode", "auth.json"): `{"openai": "private-secret"}` + "\n",
		filepath.Join(home, ".claude", ".credentials.json"):             `{"token": "private-secret"}` + "\n",
	}
	for path, text := range secrets {
		writeMode(t, path, text, 0o644)
	}
	opencode := filepath.Join(home, ".config", "opencode", "opencode.json")
	writeMode(t, opencode, `{"agent": {"mine": {"model": "openai/gpt-5.5"}}}`+"\n", 0o644)
	untouched := func(stage string) {
		t.Helper()
		if got := readFile(t, private); got != "mine\n" {
			t.Errorf("%s changed the private agent: %q", stage, got)
		}
		if !isSymlink(foreign) {
			t.Errorf("%s removed a link it did not write", stage)
		}
		for path, text := range secrets {
			if got := readFile(t, path); got != text {
				t.Errorf("%s changed %s: %q", stage, path, got)
			}
		}
		if !strings.Contains(readFile(t, opencode), `"mine"`) {
			t.Errorf("%s dropped the private OpenCode agent", stage)
		}
	}

	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	for _, rel := range agentOutputs {
		client, name, _ := strings.Cut(rel, "/")
		path := filepath.Join(roots[client], name)
		if isSymlink(path) || strings.Contains(readFileOr(path), "private-secret") || readFileOr(path) == "" {
			t.Errorf("%s is not a clean copy", path)
		}
	}
	untouched("the install")

	ok(t, runInstaller(t, base, "--uninstall", "--yes"), "uninstall")
	for _, root := range roots {
		for _, entry := range readDir(root) {
			if entry != "mine.md" && entry != "foreign.md" {
				t.Errorf("the uninstall left %s in %s", entry, root)
			}
		}
	}
	untouched("the uninstall")
}

// An unrecorded entry at a role agent path is archived before the copy lands, never overwritten in place.
func TestAgentInstallArchivesUnrecordedCollisions(t *testing.T) {
	for _, kind := range []string{"file", "foreign-link", "root-link"} {
		t.Run(kind, func(t *testing.T) {
			base := sandbox(t)
			home := filepath.Join(base, "home")
			root := filepath.Join(home, ".config", "opencode", "agents")
			private := filepath.Join(base, "private", "reviewer-b.md")
			writeMode(t, private, "private agent\n", 0o644)
			switch kind {
			case "file":
				writeMode(t, filepath.Join(root, "reviewer-b.md"), "private agent\n", 0o644)
			case "foreign-link":
				symlink(t, private, filepath.Join(root, "reviewer-b.md"))
			case "root-link":
				symlink(t, filepath.Dir(private), root)
			}
			ok(t, runInstaller(t, base, "--skip-packages"), "install")
			if got := readFile(t, private); got != "private agent\n" {
				t.Errorf("the install wrote through to %s: %q", private, got)
			}
			if installed := filepath.Join(root, "reviewer-b.md"); isSymlink(installed) || readFileOr(installed) == "private agent\n" {
				t.Errorf("%s is not the harness copy", installed)
			}
			archived, _ := filepath.Glob(filepath.Join(home, ".dotfiles-backups", "harness", "*", ".config", "opencode", "agents*"))
			if len(archived) == 0 {
				t.Error("the collision was not archived")
			}
		})
	}
}

// A copy edited after the install is the user's: the uninstall keeps it and says so.
func TestUninstallKeepsAnEditedCopy(t *testing.T) {
	base := sandbox(t)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	edited := filepath.Join(base, "home", ".claude", "commands", "ship.md")
	writeMode(t, edited, "my ship\n", 0o644)
	r := runInstaller(t, base, "--uninstall", "--yes")
	ok(t, r, "uninstall")
	if got := readFileOr(edited); got != "my ship\n" {
		t.Errorf("the uninstall removed or changed the edited copy: %q", got)
	}
	if !strings.Contains(r.stderr, "changed since the harness wrote it") {
		t.Errorf("the kept copy was not reported: %q", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(base, "home", ".harness")); err == nil {
		t.Error("a full uninstall left ~/.harness")
	}
}

// TestInstallInstallsTheEmbeddedHarnessFromAnywhere runs outside any checkout with no asset override, so what lands is what the binary carries.
func TestInstallInstallsTheEmbeddedHarnessFromAnywhere(t *testing.T) {
	base := sandbox(t)
	seedFull(t, base)
	home := filepath.Join(base, "home")
	cmd := exec.Command(binary, "install")
	cmd.Dir = base
	cmd.Env = []string{"HOME=" + home, "PATH=" + filepath.Join(base, "shims") + ":/usr/bin:/bin", "SHELL=/bin/zsh", "TMPDIR=" + filepath.Join(base, "tmp")}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if help, err := exec.Command(filepath.Join(home, ".local", "bin", "harness"), "--help").Output(); err != nil || !strings.Contains(string(help), "install") {
		t.Errorf("the installed harness does not run: %v %s", err, help)
	}
	if got, want := readFileOr(filepath.Join(home, ".claude", "CLAUDE.md")), readFile(t, filepath.Join("..", "..", "POLICY.md")); got != want {
		t.Error("install did not install the embedded policy at ~/.claude/CLAUDE.md")
	}
}

// --clear-backups reads only $HOME, so it runs from outside any checkout.
func TestClearBackupsRunsOutsideTheCheckout(t *testing.T) {
	base := sandbox(t)
	seedBackups(t, base)
	home := filepath.Join(base, "home")
	cmd := exec.Command(binary, "install", "--clear-backups")
	cmd.Dir, cmd.Env = home, []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clear-backups outside the checkout: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".dotfiles-backups", "harness")); err == nil {
		t.Error("the harness backups survived")
	}
}

// TestInstallReplacesAnUnusableStatuslineBinary keeps the tracked statusLine honest: a missing binary, one without the exec bit, or one predating the subcommand is replaced by the running harness rather than left for Claude Code to draw an empty status line with.
func TestInstallReplacesAnUnusableStatuslineBinary(t *testing.T) {
	installed := func(base string) string { return filepath.Join(base, "home", ".local", "bin", "harness") }
	cases := []struct {
		name string
		seed func(t *testing.T, base string)
	}{
		{"missing", func(t *testing.T, base string) {
			if err := os.Remove(installed(base)); err != nil {
				t.Fatal(err)
			}
		}},
		{"not executable", func(t *testing.T, base string) {
			if err := os.Chmod(installed(base), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"pre-port", func(t *testing.T, base string) {
			writeMode(t, installed(base), "#!/bin/sh\n[ \"$1\" = statusline ] && exit 64\nexit 0\n", 0o755)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := sandbox(t)
			c.seed(t, base)
			ok(t, runInstaller(t, base, "--skip-packages"), "install")
			if err := exec.Command(installed(base), "statusline", "--help").Run(); err != nil {
				t.Fatalf("the installed harness has no working statusline: %v", err)
			}
		})
	}
}

// A non-object mcp in opencode.json stops the run before either client is registered, and leaves the file as it was.
func TestAgentBrowserRefusesANonObjectOpenCodeMCP(t *testing.T) {
	base := sandbox(t)
	config := filepath.Join(base, "home", ".config", "opencode", "opencode.json")
	const text = `{"mcp": ["x"]}` + "\n"
	writeMode(t, config, text, 0o644)
	r := runInstaller(t, base, "--skip-packages")
	if r.code == 0 || !strings.Contains(r.stderr, "mcp is not a JSON object") {
		t.Fatalf("exit %d, stderr %q; want the mcp refusal", r.code, r.stderr)
	}
	if got := readFile(t, config); got != text {
		t.Errorf("the refused config changed: %q", got)
	}
	if strings.Contains(readFile(t, filepath.Join(base, "shims.log")), "claude mcp add") {
		t.Error("Claude Code was registered before the refusal")
	}
}

// A registration that landed outside the user scope fails the run, so the scope check cannot pass on a server Claude Code would not load for every project. Only a full run reaches checkTools.
func TestInstallRefusesANonUserScopeEngramMCP(t *testing.T) {
	base := sandbox(t)
	seedLived(t, base)
	writeShim(t, base, "claude", `case "$*" in
"plugin list --json") echo '[{"id": "engram@engram"}]' ;;
"mcp get agent-browser") echo 'Scope: User config' ;;
"mcp get engram") echo 'Scope: Project config' ;;
esac`)
	r := runInstaller(t, base)
	if r.code == 0 || !strings.Contains(r.stderr, "engram is not registered in the Claude Code user scope") {
		t.Fatalf("exit %d, stderr %q; want the engram scope refusal", r.code, r.stderr)
	}
}

// A surviving engram entry fails the uninstall's own sweep, so the removal at removeClaudeMCP cannot pass vacuously.
func TestUninstallRefusesASurvivingEngramMCP(t *testing.T) {
	base := sandbox(t)
	seedLived(t, base)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	writeMode(t, filepath.Join(base, "home", ".claude.json"), `{"mcpServers": {"engram": {}}}`+"\n", 0o644)
	r := runInstaller(t, base, "--uninstall", "--yes")
	if r.code == 0 || !strings.Contains(r.stderr, "The engram MCP survived the uninstall") {
		t.Fatalf("exit %d, stderr %q; want the engram survivor refusal", r.code, r.stderr)
	}
}

// TestFullInstallProvisionsMissingOnly locks the provisioning contract: one inventory per full run, no upgrade command, an install only for a formula the inventory lacks, and MCP reconciliation that still runs though the executables already exist.
func TestFullInstallProvisionsMissingOnly(t *testing.T) {
	base := sandbox(t)
	seedFull(t, base)
	ok(t, runInstaller(t, base), "full install")
	log := readFile(t, filepath.Join(base, "shims.log"))
	if got := strings.Count(log, "brew list --formula"); got != 1 {
		t.Errorf("full install inventoried formulae %d times, want 1:\n%s", got, log)
	}
	if strings.Contains(log, "brew upgrade") {
		t.Errorf("full install issued an upgrade:\n%s", log)
	}
	for _, absent := range []string{"brew install node", "brew install gh", "brew install anomalyco/tap/opencode"} {
		if strings.Contains(log, absent) {
			t.Errorf("full install installed the already-present %q:\n%s", absent, log)
		}
	}
	for _, want := range []string{"brew install uv", "brew install worktrunk", "brew install agent-browser", "brew install gentleman-programming/tap/engram"} {
		if !strings.Contains(log, want) {
			t.Errorf("full install skipped the missing %q:\n%s", want, log)
		}
	}
	for _, want := range []string{"engram setup claude-code", "engram setup opencode", "claude mcp add --scope user agent-browser", "claude mcp add --scope user engram"} {
		if !strings.Contains(log, want) {
			t.Errorf("full install skipped reconciliation though the tools exist, missing %q", want)
		}
	}
}

// TestSkipPackagesCallsNoPackageManager locks that the option named for skipping packages makes no package-manager call while still reconciling the links and the MCP registrations.
func TestSkipPackagesCallsNoPackageManager(t *testing.T) {
	base := sandbox(t)
	seedLived(t, base)
	ok(t, runInstaller(t, base, "--skip-packages"), "skip-packages install")
	log := readFile(t, filepath.Join(base, "shims.log"))
	if strings.Contains(log, "brew ") {
		t.Errorf("--skip-packages called the package manager:\n%s", log)
	}
	for _, want := range []string{"engram setup claude-code", "claude mcp add --scope user agent-browser", "claude mcp add --scope user engram"} {
		if !strings.Contains(log, want) {
			t.Errorf("--skip-packages skipped reconciliation, missing %q", want)
		}
	}
	link := filepath.Join(base, "home", ".claude", "CLAUDE.md")
	if got, want := readFileOr(link), readFile(t, filepath.Join(base, "checkout", "POLICY.md")); got != want {
		t.Errorf("--skip-packages did not install the policy at %s", link)
	}
}

// TestInstallAddsClaudeOnlyWhenAbsent covers the branch no golden reaches: every sandbox has the claude shim, so this run removes it and stands in for Anthropic's installer, which has to be reached exactly once.
func TestInstallAddsClaudeOnlyWhenAbsent(t *testing.T) {
	base := sandbox(t)
	seedFull(t, base)
	shims, log := filepath.Join(base, "shims"), filepath.Join(base, "shims.log")
	claude := filepath.Join(shims, "claude")
	saved := filepath.Join(base, "claude.saved")
	writeMode(t, saved, readFile(t, claude), 0o755)
	if err := os.Remove(claude); err != nil {
		t.Fatal(err)
	}
	// bash stands in for the native installer: it records the call and writes the claude binary the tool check and the agent-browser registration then look for.
	writeMode(t, filepath.Join(shims, "bash"), "#!/bin/sh\nprintf '%s\\n' \"bash $*\" >> '"+log+"'\ncp '"+saved+"' '"+claude+"'\nchmod +x '"+claude+"'\nexit 0\n", 0o755)

	ok(t, runInstaller(t, base), "full install without claude")

	logged := readFile(t, log)
	if got := strings.Count(logged, "claude.ai/install.sh"); got != 1 {
		t.Errorf("claude absent ran the native installer %d times, want exactly 1:\n%s", got, logged)
	}
	if _, err := os.Stat(claude); err != nil {
		t.Errorf("the native installer left claude off PATH: %v", err)
	}
}

// TestInstallRefusesMissingOrRelativeLocations proves the installer validates HOME and the config location before any write, so a relative one can never turn managed paths into writes inside the invocation directory.
func TestInstallRefusesMissingOrRelativeLocations(t *testing.T) {
	for _, c := range []struct {
		name string
		env  []string
	}{
		{"missing-home", []string{"HOME="}},
		{"relative-home", []string{"HOME=relative"}},
		{"relative-config", []string{"XDG_CONFIG_HOME=relative"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			base := sandbox(t)
			home := filepath.Join(base, "home")
			before := homeTree(t, home)
			r := runHarness(t, base, c.env, "install", "--skip-packages")
			if r.code == 0 || !strings.Contains(r.stderr, "must be absolute") {
				t.Fatalf("exit %d, stderr %q; want a refusal naming the absolute-path requirement", r.code, r.stderr)
			}
			if got := homeTree(t, home); got != before {
				t.Errorf("a refused run wrote into the home tree:\n%s", got)
			}
			if raw, err := os.ReadFile(filepath.Join(base, "shims.log")); err == nil && strings.TrimSpace(string(raw)) != "" {
				t.Errorf("a refused run spawned a subprocess:\n%s", raw)
			}
		})
	}
}

// homeTree renders the home tree so a test can assert a refused run left it unchanged.
func homeTree(t *testing.T, home string) string {
	t.Helper()
	var b strings.Builder
	tree(t, &b, home, true)
	return b.String()
}

// A second install over unchanged copies rewrites and archives nothing.
func TestReinstallRewritesNothingUnchanged(t *testing.T) {
	base := sandbox(t)
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	r := runInstaller(t, base, "--skip-packages")
	ok(t, r, "reinstall")
	for _, noise := range []string{"Installed ", "Backed up ", "Removed "} {
		if strings.Contains(r.stdout, noise) {
			t.Errorf("the reinstall logged %q:\n%s", noise, r.stdout)
		}
	}
}

// A TMPDIR reached through a symlink, as macOS /var is, still extracts assets the role agent renderer accepts.
func TestInstallWithSymlinkedTmpdir(t *testing.T) {
	base := sandbox(t)
	link := filepath.Join(base, "tmp-link")
	symlink(t, filepath.Join(base, "tmp"), link)
	r := runHarnessInput(t, base, "", []string{"TMPDIR=" + link, "HARNESS_ASSETS=" + filepath.Join(base, "checkout")}, "install", "--skip-packages")
	ok(t, r, "install")
}
