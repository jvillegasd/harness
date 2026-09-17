package dash

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/checkout"
)

// project lays out a git repository, with the setup-harness configuration when config is set.
func project(t *testing.T, config string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if config != "" {
		write(t, filepath.Join(root, harnessConfig), config)
	}
	return root
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

const localConfig = `{"version": 1, "tracker": "local-markdown", "spec_home": "docs/specs", "labels": {"blocked": "blocked", "ready_for_agent": "ready-for-agent", "in_progress": "in-progress", "ready_for_human": "ready-for-human"}}`

// The dashboard runs in any repository setup-harness configured, from any directory inside it, and refuses one it did not.
func TestProjectRootNeedsTheSetupHarnessConfig(t *testing.T) {
	bare := project(t, "")
	if _, err := projectRoot(bare); err == nil || !strings.Contains(err.Error(), "setup-harness") {
		t.Errorf("an unconfigured repository resolved: %v", err)
	}
	if _, err := projectRoot(t.TempDir()); err == nil {
		t.Error("a directory outside git resolved")
	}
	root := project(t, localConfig)
	nested := filepath.Join(root, "src", "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := projectRoot(nested); err != nil || got != root {
		t.Errorf("projectRoot(nested) = %q, %v; want %q", got, err, root)
	}
}

// Outside the harness checkout the lock and bench panels say so instead of reading files the project does not have.
func TestHarnessOnlyPanelsOutsideTheCheckout(t *testing.T) {
	dir := checkout.Checkout{Root: project(t, localConfig)}
	if _, err := loadLocks(dir); !errors.Is(err, errHarnessOnly) {
		t.Errorf("loadLocks = %v, want %v", err, errHarnessOnly)
	}
	if _, err := loadBench(dir); !errors.Is(err, errHarnessOnly) {
		t.Errorf("loadBench = %v, want %v", err, errHarnessOnly)
	}
}

// A local-markdown tracker is counted from the Status: lines of its tickets and promoted specs.
func TestTrackerCountsLocalMarkdownStatuses(t *testing.T) {
	root := project(t, localConfig)
	specs := filepath.Join(root, "docs", "specs")
	write(t, filepath.Join(specs, "login", "spec.md"), "Status: in-progress\n# Login\n")
	write(t, filepath.Join(specs, "login", "issues", "01.md"), "Status: ready-for-agent\nBlocked by: none\n## Type\n")
	write(t, filepath.Join(specs, "login", "issues", "02.md"), "Status: blocked\nBlocked by: 01\n## Type\n")
	write(t, filepath.Join(specs, "search", "spec.md"), "Status: ready-for-agent\nBlocked by: none\n# Search\n")
	write(t, filepath.Join(specs, "search", "notes.md"), "Status: blocked\n")
	got := loadTracker(checkout.Checkout{Root: root})
	if got.Err != "" {
		t.Fatal(got.Err)
	}
	want := map[string]int{"blocked": 1, "ready-for-agent": 2, "in-progress": 1, "ready-for-human": 0}
	for _, label := range got.Labels {
		if label.Count != want[label.Label] {
			t.Errorf("%s = %d, want %d", label.Label, label.Count, want[label.Label])
		}
	}
}

func TestTrackerRefusesAnUnknownMode(t *testing.T) {
	root := project(t, strings.Replace(localConfig, "local-markdown", "jira", 1))
	if got := loadTracker(checkout.Checkout{Root: root}); !strings.Contains(got.Err, `"jira"`) {
		t.Errorf("Err = %q, want the unknown mode named", got.Err)
	}
}
