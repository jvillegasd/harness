package resume

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	write(t, filepath.Join(dir, "docs/agents/tasks/old.md"), "# Old\n\n## State and next action\n\nShipped.\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "base")
	runGit(t, dir, "switch", "-q", "-c", "feat/x")
	return dir
}

func TestContextNamesTheBranchAndItsTaskDocument(t *testing.T) {
	dir := repo(t)
	write(t, filepath.Join(dir, "docs/agents/tasks/x.md"), "# X\n\n## Requirements\n\nkeep out\n\n## State and next action\n\nNext: wire the hook.\n\n## Decisions\n\nkeep out too\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "wip: hook")
	got := Context(filepath.Join(dir, "docs"))
	for _, want := range []string{"- branch: feat/x", "wip: hook", "- task document: docs/agents/tasks/x.md", "## State and next action", "Next: wire the hook."} {
		if !strings.Contains(got, want) {
			t.Errorf("context lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"keep out", "old.md"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("context carries %q, which is outside the changed document's next action:\n%s", unwanted, got)
		}
	}
}

func TestContextListsAnUntrackedTaskDocument(t *testing.T) {
	dir := repo(t)
	write(t, filepath.Join(dir, "docs/agents/tasks/new.md"), "## State and next action\n\nStart.\n")
	if got := Context(dir); !strings.Contains(got, "docs/agents/tasks/new.md") || !strings.Contains(got, "Start.") {
		t.Errorf("an untracked task document is missing:\n%s", got)
	}
}

func TestContextCapsALongSection(t *testing.T) {
	dir := repo(t)
	write(t, filepath.Join(dir, "docs/agents/tasks/long.md"), "## State and next action\n"+strings.Repeat("line\n", 200))
	if got := Context(dir); strings.Count(got, "line\n") > maxSectionLines || !strings.Contains(got, "[...]") {
		t.Errorf("a long section was not capped:\n%s", got)
	}
}

func TestContextIsEmptyOutsideACheckout(t *testing.T) {
	if got := Context(t.TempDir()); got != "" {
		t.Errorf("outside a checkout the context is %q, want nothing", got)
	}
}

func TestExcludeKeepsTaskDocumentsLocalAndListed(t *testing.T) {
	dir := repo(t)
	for range 2 {
		if err := Exclude(dir); err != nil {
			t.Fatal(err)
		}
	}
	exclude, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(exclude), "/docs/agents/tasks/\n"); n != 1 {
		t.Fatalf("exclude holds the folder %d times, want once:\n%s", n, exclude)
	}
	write(t, filepath.Join(dir, "docs/agents/tasks/local.md"), "## State and next action\n\nKeep going.\n")
	runGit(t, dir, "add", "-A")
	if out, _ := exec.Command("git", "-C", dir, "diff", "--cached", "--name-only").Output(); strings.Contains(string(out), "local.md") {
		t.Fatalf("an excluded task document was staged:\n%s", out)
	}
	if got := Context(dir); !strings.Contains(got, "docs/agents/tasks/local.md") || !strings.Contains(got, "Keep going.") {
		t.Errorf("an excluded task document is missing from the context:\n%s", got)
	}
}
