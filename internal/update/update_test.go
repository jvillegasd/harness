package update

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingToolsAreSkippedOutsideCheckout(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Chdir(t.TempDir())
	var out, err bytes.Buffer
	if code := Command([]string{"clients"}, &out, &err); code != 0 {
		t.Fatalf("exit %d: %s", code, &err)
	}
	for _, name := range []string{"claude", "codex", "opencode"} {
		if !strings.Contains(out.String(), "skipped: "+name) {
			t.Errorf("missing skip for %s: %s", name, &out)
		}
	}
}

func shim(t *testing.T, path, script string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestBrewOwnershipAndFailureContinuation(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	prefix := filepath.Join(root, "brew")
	log := filepath.Join(root, "calls")
	t.Setenv("PATH", bin)
	t.Setenv("HOME", root)
	t.Chdir(root)
	for _, name := range []string{"node", "gh", "uv"} {
		path := filepath.Join(prefix, "Cellar", name, "1/bin", name)
		shim(t, path, "echo 1\n")
		os.MkdirAll(bin, 0755)
		if err := os.Symlink(path, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	shim(t, filepath.Join(bin, "brew"), `
 echo "$*" >> '`+log+`'
 case "$*" in
 '--prefix') echo '`+prefix+`';;
 'info --json=v2 --installed') echo '{"formulae":[{"name":"node","full_name":"node","installed":[{"version":"1"}]},{"name":"gh","full_name":"gh","installed":[{"version":"1"}]},{"name":"uv","full_name":"uv","installed":[{"version":"1"}]}],"casks":[]}';;
 'upgrade --formula node') exit 1;;
 esac
 `)
	var out, err bytes.Buffer
	if code := Command([]string{"packages"}, &out, &err); code != 1 {
		t.Fatalf("exit %d: %s %s", code, &out, &err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "upgrade --formula gh") || !strings.Contains(string(calls), "upgrade --formula uv") {
		t.Fatalf("did not continue: %s", calls)
	}
	if strings.Count(string(calls), "info --json=v2 --installed") != 1 {
		t.Fatalf("inventory: %s", calls)
	}
	if !strings.Contains(out.String(), "failed: node") || !strings.Contains(out.String(), "unchanged: gh") {
		t.Fatal(out.String())
	}
}

func TestMCPPrerequisiteFailureSkipsRuntimeAndRegistrations(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	log := filepath.Join(root, "calls")
	t.Setenv("HOME", root)
	t.Setenv("PATH", bin)
	t.Chdir(root)
	shim(t, filepath.Join(bin, "brew"), `echo "$*" >> '`+log+`'
 case "$*" in
 '--prefix') echo '`+root+`';;
 'info --json=v2 --installed') echo '{"formulae":[{"name":"agent-browser","installed":[{"version":"1"}]}]}';;
 'upgrade --formula agent-browser') exit 1;;
 esac`)
	tool := filepath.Join(root, "Cellar/agent-browser/1/bin/agent-browser")
	shim(t, tool, `echo "browser $*" >> '`+log+`'; echo 1`)
	os.Symlink(tool, filepath.Join(bin, "agent-browser"))
	shim(t, filepath.Join(bin, "claude"), `echo "claude $*" >> '`+log+`'`)
	var out, err bytes.Buffer
	if code := Command([]string{"mcps"}, &out, &err); code != 1 {
		t.Fatalf("%d %s %s", code, &out, &err)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "browser install") || strings.Contains(string(calls), "claude mcp") {
		t.Fatalf("dependent actions ran: %s", calls)
	}
	if !strings.Contains(out.String(), "skipped: agent-browser runtime") {
		t.Fatal(out.String())
	}
}

func TestClientsPreserveNativeNpmAndExternalOwnership(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	log := filepath.Join(root, "calls")
	prefix := filepath.Join(root, "npm-owner")
	t.Setenv("HOME", root)
	t.Setenv("PATH", bin)
	t.Chdir(root)
	native := filepath.Join(root, ".local/share/claude/versions/1")
	codex := filepath.Join(prefix, "lib/node_modules/@openai/codex/bin/codex.js")
	external := filepath.Join(root, "Library/Application Support/Orca/clients/opencode")
	for _, item := range []struct{ name, path string }{{"claude", native}, {"codex", codex}, {"opencode", external}} {
		shim(t, item.path, `echo "`+item.name+` $*" >> '`+log+`'; echo 1`)
		os.MkdirAll(bin, 0755)
		os.Symlink(item.path, filepath.Join(bin, item.name))
	}
	shim(t, filepath.Join(bin, "npm"), `echo "npm $*" >> '`+log+`'
 if [ "$1" = ls ]; then echo '{"dependencies":{"@openai/codex":{"version":"1.0.0"}}}'; fi`)
	var out, err bytes.Buffer
	if code := Command([]string{"clients"}, &out, &err); code != 0 {
		t.Fatalf("%d %s %s", code, &out, &err)
	}
	calls, _ := os.ReadFile(log)
	for _, want := range []string{"claude update", "npm install --global --prefix " + resolved(prefix) + " @openai/codex@latest"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("missing %s: %s", want, calls)
		}
	}
	if strings.Contains(string(calls), "opencode ") || !strings.Contains(out.String(), "owned by Orca") {
		t.Fatalf("external client: %s %s", calls, &out)
	}
}

func TestIDEsKeepPreviewAndUseManualFallback(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	log := filepath.Join(root, "calls")
	t.Setenv("PATH", bin)
	t.Setenv("HOME", root)
	t.Chdir(root)
	shim(t, filepath.Join(bin, "brew"), `echo "$*" >> '`+log+`'
 case "$*" in
 '--prefix') echo '`+root+`';;
 'upgrade --help') echo '--no-quit';;
 'info --json=v2 --installed') echo '{"casks":[{"token":"zed@preview","installed":"1"}]}';;
 esac`)
	os.MkdirAll(filepath.Join(root, "Applications/Orca.app"), 0755)
	var out, err bytes.Buffer
	if code := Command([]string{"ides"}, &out, &err); code != 0 {
		t.Fatalf("%d: %s %s", code, &out, &err)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "upgrade --cask --no-quit zed@preview") || strings.Contains(string(calls), "upgrade --cask --no-quit zed\n") {
		t.Fatal(string(calls))
	}
	if !strings.Contains(out.String(), "manual: orca") {
		t.Fatal(out.String())
	}
}

func TestMCPRefreshTouchesOnlyRegistrations(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	log := filepath.Join(root, "calls")
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	t.Setenv("PATH", bin)
	t.Chdir(root)
	shim(t, filepath.Join(bin, "brew"), `echo "brew $*" >> '`+log+`'
 case "$*" in
 '--prefix') echo '`+root+`';;
 'info --json=v2 --installed') echo '{"formulae":[{"name":"agent-browser","installed":[{"version":"1"}]},{"name":"engram","full_name":"gentleman-programming/tap/engram","installed":[{"version":"1"}]}]}';;
 esac`)
	for _, name := range []string{"engram", "agent-browser"} {
		path := filepath.Join(root, "Cellar", name, "1/bin", name)
		shim(t, path, `echo "`+name+` $*" >> '`+log+`'; echo 1`)
		os.Symlink(path, filepath.Join(bin, name))
	}
	for _, name := range []string{"claude", "codex", "opencode"} {
		shim(t, filepath.Join(bin, name), `echo "`+name+` $*" >> '`+log+`'`)
	}
	config := filepath.Join(root, ".config/opencode/opencode.json")
	os.MkdirAll(filepath.Dir(config), 0755)
	os.WriteFile(config, []byte(`{"keep":true,"mcp":{"private":{"type":"local","command":["private"]}}}`), 0644)
	var out, err bytes.Buffer
	if code := Command([]string{"mcps"}, &out, &err); code != 0 {
		t.Fatalf("%d %s %s", code, &out, &err)
	}
	calls, _ := os.ReadFile(log)
	for _, want := range []string{"agent-browser install", "claude mcp add --scope user engram --", "codex mcp add agent-browser --", "brew upgrade --formula gentleman-programming/tap/engram"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("missing %s: %s", want, calls)
		}
	}
	for _, bad := range []string{" setup ", " plugin ", " restart", " launch"} {
		if strings.Contains(string(calls), bad) {
			t.Fatalf("unrelated operation %s: %s", bad, calls)
		}
	}
	data, _ := os.ReadFile(config)
	if !strings.Contains(string(data), `"keep": true`) || !strings.Contains(string(data), `"private"`) || !strings.Contains(string(data), `"engram"`) {
		t.Fatalf("config: %s", data)
	}
	for _, path := range []string{".harness", ".claude/settings.json", ".codex/AGENTS.md", ".zshrc"} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("unrelated asset written: %s", path)
		}
	}
}

func TestExplicitSkillsOutsideCheckoutDoesNotDiscoverMachine(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	log := filepath.Join(root, "calls")
	t.Setenv("HOME", root)
	t.Setenv("PATH", bin)
	t.Chdir(root)
	shim(t, filepath.Join(bin, "brew"), `echo ran > '`+log+`'`)
	var out, err bytes.Buffer
	if code := Command([]string{"skills"}, &out, &err); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("machine discovery ran")
	}
}

func TestBareUpdateDiscoversFirstDeduplicatesAndKeepsIDEsLast(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	log := filepath.Join(root, "calls")
	v := filepath.Join(root, "version")
	t.Setenv("PATH", bin)
	t.Setenv("HOME", root)
	t.Chdir(root)
	os.WriteFile(v, []byte("1\n"), 0644)
	node := filepath.Join(root, "Cellar/node@22/1/bin/node")
	shim(t, node, `/bin/cat '`+v+`'`)
	os.MkdirAll(bin, 0755)
	os.Symlink(node, filepath.Join(bin, "node"))
	shim(t, filepath.Join(bin, "brew"), `echo "$*" >> '`+log+`'
 case "$*" in
 '--prefix') echo '`+root+`';;
 'info --json=v2 --installed') echo '{"formulae":[{"name":"node@22","installed":[{"version":"1"}]}],"casks":[{"token":"orca","installed":"1"},{"token":"orca","installed":"1"}]}';;
 'upgrade --help') echo '--no-quit';;
 'upgrade --formula node@22') echo 2 > '`+v+`';;
 esac`)
	var out, err bytes.Buffer
	if code := Command(nil, &out, &err); code != 0 {
		t.Fatalf("%d %s %s", code, &out, &err)
	}
	calls, _ := os.ReadFile(log)
	s := string(calls)
	if strings.Count(s, "upgrade --cask --no-quit orca") != 1 {
		t.Fatalf("duplicate target: %s", s)
	}
	if strings.Index(s, "upgrade --help") > strings.Index(s, "upgrade --formula node@22") || strings.Index(s, "upgrade --formula node@22") > strings.Index(s, "upgrade --cask --no-quit orca") {
		t.Fatalf("wrong order: %s", s)
	}
	if !strings.Contains(out.String(), "updated: node: 1 -> 2") || !strings.Contains(out.String(), "skipped: skills") {
		t.Fatal(out.String())
	}
}

func TestUnsupportedOwnershipAndPrereleaseAreManual(t *testing.T) {
	for _, v := range []string{"1.0.0-alpha.1", ""} {
		t.Run(v, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			log := filepath.Join(root, "calls")
			t.Setenv("HOME", root)
			t.Setenv("PATH", bin)
			t.Chdir(root)
			path := filepath.Join(root, "lib/node_modules/@openai/codex/bin/codex.js")
			shim(t, path, "exit 0")
			os.MkdirAll(bin, 0755)
			os.Symlink(path, filepath.Join(bin, "codex"))
			shim(t, filepath.Join(bin, "npm"), `echo "$*" >> '`+log+`'; echo '{"dependencies":{"@openai/codex":{"version":"`+v+`"}}}'`)
			shim(t, filepath.Join(bin, "claude"), "exit 42")
			var out, err bytes.Buffer
			if code := Command([]string{"clients"}, &out, &err); code != 0 {
				t.Fatal(code, out.String(), err.String())
			}
			calls, _ := os.ReadFile(log)
			if strings.Contains(string(calls), "install") || !strings.Contains(out.String(), "manual: codex") || !strings.Contains(out.String(), "manual: claude") {
				t.Fatal(string(calls), out.String())
			}
		})
	}
}

func TestUpgradePreservesHomebrewRefreshPolicy(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	t.Setenv("HOME", root)
	t.Setenv("PATH", bin)
	t.Setenv("HOMEBREW_NO_AUTO_UPDATE", "")
	path := filepath.Join(root, "Cellar/gh/1/bin/gh")
	shim(t, path, "echo 1")
	os.MkdirAll(bin, 0755)
	os.Symlink(path, filepath.Join(bin, "gh"))
	shim(t, filepath.Join(bin, "brew"), `
 case "$*" in
 '--prefix') echo '`+root+`';;
 'info --json=v2 --installed') echo '{"formulae":[{"name":"gh","installed":[{"version":"1"}]}]}';;
 'upgrade --formula gh') [ -z "$HOMEBREW_NO_AUTO_UPDATE" ] || exit 9;;
 esac`)
	var out, err bytes.Buffer
	if code := Command([]string{"packages"}, &out, &err); code != 0 {
		t.Fatalf("refresh policy overridden: %d %s %s", code, &out, &err)
	}
}
