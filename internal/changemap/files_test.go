package changemap

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// gitIn runs git in dir isolated from the user's config, like the command tests do.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(cfg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+cfg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(bytes.TrimSpace(out))
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a repository on main with a base commit holding the files the listing tests then change.
func newRepo(t *testing.T) string {
	t.Helper()
	// Git config is isolated per call, so the environment variables must also reach Root and Changed.
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(cfg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "keep.go", "package a\n")
	write(t, dir, "gone.sql", "select 1;\n")
	write(t, dir, "edit.go", "package a\n")
	write(t, dir, "sub/staged.go", "package a\n")
	write(t, dir, "sub/unstaged.go", "package a\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func TestChangedListsEveryKindOfChange(t *testing.T) {
	dir := newRepo(t)
	gitIn(t, dir, "switch", "-q", "-c", "feat")
	// Committed on the branch.
	write(t, dir, "edit.go", "package a\n// changed\n")
	write(t, dir, "committed.go", "package a\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "branch work")
	// Staged.
	write(t, dir, "sub/staged.go", "package a\n// staged\n")
	gitIn(t, dir, "add", "sub/staged.go")
	// Unstaged.
	write(t, dir, "sub/unstaged.go", "package a\n// unstaged\n")
	// Untracked, not ignored.
	write(t, dir, "new dir/untracked file.go", "package a\n")
	write(t, dir, ".gitignore", "ignored.log\n")
	write(t, dir, "ignored.log", "x")
	// Deleted.
	if err := os.Remove(filepath.Join(dir, "gone.sql")); err != nil {
		t.Fatal(err)
	}

	changes, err := Changed(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".gitignore", "committed.go", "edit.go", "gone.sql", "new dir/untracked file.go", "sub/staged.go", "sub/unstaged.go"}
	if !reflect.DeepEqual(changes.Files, want) {
		t.Fatalf("Files = %q, want %q", changes.Files, want)
	}
	if changes.Base != "main" {
		t.Errorf("Base = %q, want main", changes.Base)
	}
	if mb := gitIn(t, dir, "merge-base", "HEAD", "main"); changes.MergeBase != mb {
		t.Errorf("MergeBase = %q, want %q", changes.MergeBase, mb)
	}
}

func TestChangedUsesMergeBaseNotBaseTip(t *testing.T) {
	dir := newRepo(t)
	gitIn(t, dir, "switch", "-q", "-c", "feat")
	write(t, dir, "feature.go", "package a\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "feature")
	// The base moves on after the branch point; its work is not ours.
	gitIn(t, dir, "switch", "-q", "main")
	write(t, dir, "upstream.go", "package a\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "upstream")
	gitIn(t, dir, "switch", "-q", "feat")

	changes, err := Changed(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"feature.go"}; !reflect.DeepEqual(changes.Files, want) {
		t.Fatalf("Files = %q, want %q", changes.Files, want)
	}
}

func TestChangedFromSubdirectoryReportsRootRelativePaths(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "sub/untracked.go", "package a\n")
	write(t, dir, "edit.go", "package a\n// changed\n")
	root, err := Root(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	resolved, _ := filepath.EvalSymlinks(dir)
	got, _ := filepath.EvalSymlinks(root)
	if got != resolved {
		t.Fatalf("Root = %q, want %q", got, resolved)
	}
	changes, err := Changed(root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"edit.go", "sub/untracked.go"}; !reflect.DeepEqual(changes.Files, want) {
		t.Fatalf("Files = %q, want %q", changes.Files, want)
	}
}

func TestChangedEmptyWhenNothingChanged(t *testing.T) {
	dir := newRepo(t)
	changes, err := Changed(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Files) != 0 {
		t.Fatalf("Files = %q, want none", changes.Files)
	}
}

func TestChangedUnknownBase(t *testing.T) {
	dir := newRepo(t)
	for _, base := range []string{"nope", "-x", "--output=evil"} {
		if _, err := Changed(dir, base); !errors.Is(err, ErrUnknownBase) {
			t.Errorf("Changed(%q) = %v, want ErrUnknownBase", base, err)
		}
	}
}

func TestRootOutsideRepository(t *testing.T) {
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	if _, err := Root(t.TempDir()); !errors.Is(err, ErrNotRepo) {
		t.Fatalf("Root = %v, want ErrNotRepo", err)
	}
}

func TestDefaultBase(t *testing.T) {
	dir := newRepo(t)
	if got := DefaultBase(dir); got != "main" {
		t.Fatalf("DefaultBase without origin/HEAD = %q, want main", got)
	}
	// Give the repository an origin whose HEAD resolves.
	origin := t.TempDir()
	gitIn(t, origin, "init", "-q", "--bare", "-b", "main")
	gitIn(t, dir, "remote", "add", "origin", origin)
	gitIn(t, dir, "push", "-q", "origin", "main")
	gitIn(t, dir, "remote", "set-head", "origin", "main")
	if got := DefaultBase(dir); got != "origin/HEAD" {
		t.Fatalf("DefaultBase with origin/HEAD = %q, want origin/HEAD", got)
	}
}

// TestBaseConfigKeepsADroppedCriticalPath is the floor guarantee: a branch that removes an entry from criticalPaths cannot lower the tier of the file that entry protected, because the base commit's entries still count.
func TestBaseConfigKeepsADroppedCriticalPath(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, ConfigPath, `{"criticalPaths": ["POLICY.md"]}`)
	write(t, dir, "POLICY.md", "rules\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "protect policy")
	gitIn(t, dir, "switch", "-q", "-c", "branch")
	write(t, dir, ConfigPath, `{"tracker": "github"}`)
	write(t, dir, "POLICY.md", "weaker rules\n")
	changes, err := Changed(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := EffectiveConfig(dir, changes.MergeBase)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"POLICY.md", ConfigPath} {
		if got := Classify(file, cfg); got.Tier != Critical {
			t.Errorf("Classify(%q) = %v after the branch dropped its criticalPaths entry, want critical", file, got)
		}
	}
}

// TestEffectiveConfigUnionsBaseAndWorkingTree keeps entries the branch adds as well as entries the base had.
func TestEffectiveConfigUnionsBaseAndWorkingTree(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, ConfigPath, `{"criticalPaths": ["base/**"]}`)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "config")
	write(t, dir, ConfigPath, `{"criticalPaths": ["base/**", "added/**"]}`)
	base := gitIn(t, dir, "rev-parse", "HEAD")
	cfg, err := EffectiveConfig(dir, base)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"base/**", "added/**"}; !reflect.DeepEqual(cfg.CriticalPaths, want) {
		t.Errorf("EffectiveConfig = %v, want %v", cfg.CriticalPaths, want)
	}
}

// TestChosenBaseCannotLowerTheFloor covers a branch that drops a criticalPaths entry in its own first commit and then names that commit as the base: the default branch's entries still count.
func TestChosenBaseCannotLowerTheFloor(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, ConfigPath, `{"criticalPaths": ["POLICY.md"]}`)
	write(t, dir, "POLICY.md", "rules\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "protect policy")
	gitIn(t, dir, "switch", "-q", "-c", "branch")
	write(t, dir, ConfigPath, `{"tracker": "github"}`)
	gitIn(t, dir, "commit", "-q", "-am", "drop protection")
	write(t, dir, "POLICY.md", "weaker rules\n")
	changes, err := Changed(dir, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := EffectiveConfig(dir, changes.MergeBase)
	if err != nil {
		t.Fatal(err)
	}
	if got := Classify("POLICY.md", cfg); got.Tier != Critical {
		t.Errorf("Classify(POLICY.md) = %v with the branch's own commit as base, want critical", got)
	}
}
