package skills

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/checkout"
)

// localCheckout is a Git checkout with the support and ops groups and an empty lock.
func localCheckout(t *testing.T) checkout.Checkout {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	c := checkout.Checkout{Root: t.TempDir()}
	for _, g := range []string{"support", "ops"} {
		if err := os.MkdirAll(filepath.Join(c.Skills(), g), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(c.Lock(), []byte("{\n  \"version\": 1,\n  \"skills\": {}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, c.Root, "init", "-q")
	return c
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// linksResolve asserts both bookkeeping links of name reach body.
func linksResolve(t *testing.T, c checkout.Checkout, name, body string) {
	t.Helper()
	for _, link := range []string{filepath.Join(c.AgentSkills(), name), filepath.Join(c.ClaudeSkills(), name)} {
		got, err := filepath.EvalSymlinks(link)
		want, _ := filepath.EvalSymlinks(body)
		if err != nil || got != want {
			t.Errorf("%s resolves to %q (%v), want %s", link, got, err, want)
		}
	}
}

func TestNewCreatesALinkedSkillWithAllGuardsOrNone(t *testing.T) {
	c := localCheckout(t)
	if _, err := New(c, "demo", "support", false); err != nil {
		t.Fatal(err)
	}
	if _, err := New(c, "gated", "ops", true); err != nil {
		t.Fatal(err)
	}
	linksResolve(t, c, "demo", filepath.Join(c.Skills(), "support", "demo"))
	linksResolve(t, c, "gated", filepath.Join(c.Skills(), "ops", "gated"))
	raw, _ := os.ReadFile(filepath.Join(c.Skills(), "support", "demo", "SKILL.md"))
	if claude, opencode := guarded(string(raw)); claude || opencode {
		t.Errorf("an unguarded skill carries a guard:\n%s", raw)
	}
	if _, err := os.Stat(filepath.Join(c.Skills(), "support", "demo", "agents")); !os.IsNotExist(err) {
		t.Error("an unguarded skill has agents/openai.yaml")
	}
	raw, _ = os.ReadFile(filepath.Join(c.Skills(), "ops", "gated", "SKILL.md"))
	sidecar, _ := os.ReadFile(filepath.Join(c.Skills(), "ops", "gated", "agents", "openai.yaml"))
	if claude, opencode := guarded(string(raw)); !claude || !opencode || !codexGuarded(string(sidecar)) {
		t.Errorf("a guarded skill lacks a guard:\n%s\n%s", raw, sidecar)
	}
	if problems, err := Unguarded(c); err != nil || len(problems) != 0 {
		t.Errorf("guard check: %v %v", problems, err)
	}
	if results, err := Verify(c); err != nil || len(results) != 0 {
		t.Errorf("verify: %v %v", results, err)
	}
}

func TestNewRefusesAnExistingNameOrUnknownGroupAndWritesNothing(t *testing.T) {
	c := localCheckout(t)
	if _, err := New(c, "demo", "support", false); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][2]string{{"demo", "ops"}, {"other", "nope"}} {
		if _, err := New(c, args[0], args[1], false); err == nil {
			t.Errorf("new %v was accepted", args)
		}
	}
	if _, err := os.Lstat(filepath.Join(c.Skills(), "ops", "demo")); !os.IsNotExist(err) {
		t.Error("a refused new wrote a body")
	}
	if _, err := os.Lstat(filepath.Join(c.AgentSkills(), "other")); !os.IsNotExist(err) {
		t.Error("a refused new wrote a link")
	}
}

func TestMoveKeepsHistoryLinksAndTheLock(t *testing.T) {
	c := localCheckout(t)
	if _, err := New(c, "demo", "support", false); err != nil {
		t.Fatal(err)
	}
	body := filepath.Join(c.Skills(), "support", "demo")
	hash, err := folderHash(body)
	if err != nil {
		t.Fatal(err)
	}
	lock := "{\n  \"version\": 1,\n  \"skills\": {\n    \"demo\": {\n      \"source\": \"o/r\",\n      \"sourceType\": \"github\",\n      \"computedHash\": \"" + hash + "\"\n    }\n  }\n}\n"
	if err := os.WriteFile(c.Lock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, c.Root, "add", ".")
	gitIn(t, c.Root, "commit", "-q", "-m", "add demo")

	for _, g := range []string{"support", "nope"} {
		if _, err := Move(c, "demo", g); err == nil {
			t.Errorf("move to %s was accepted", g)
		}
	}
	if _, err := os.Stat(body); err != nil {
		t.Fatal("a refused move changed the body")
	}

	if _, err := Move(c, "demo", "ops"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(c.Skills(), "ops", "demo")
	linksResolve(t, c, "demo", moved)
	if _, err := os.Lstat(body); !os.IsNotExist(err) {
		t.Error("the old group still holds the body")
	}
	gitIn(t, c.Root, "commit", "-q", "-m", "move demo")
	if log := gitIn(t, c.Root, "log", "--follow", "--format=%s", "--", "skills/ops/demo/SKILL.md"); log != "move demo\nadd demo" {
		t.Errorf("history did not follow the move: %q", log)
	}
	results, err := Verify(c)
	if err != nil || len(results) != 1 || results[0].Status != "match" {
		t.Errorf("verify after the move: %+v %v", results, err)
	}
}

func TestMoveRefusesAnUnexpectedLinkBeforeMovingAnything(t *testing.T) {
	c := localCheckout(t)
	if _, err := New(c, "demo", "support", false); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(c.ClaudeSkills(), "demo")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(link, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Move(c, "demo", "ops"); err == nil {
		t.Fatal("a move over a real .claude/skills directory was accepted")
	}
	if _, err := os.Stat(filepath.Join(c.Skills(), "support", "demo", "SKILL.md")); err != nil {
		t.Error("a refused move moved the body")
	}
	linksResolveAgent := filepath.Join(c.AgentSkills(), "demo")
	if _, err := os.Readlink(linksResolveAgent); err != nil {
		t.Error("a refused move removed the .agents link")
	}
}
