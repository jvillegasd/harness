package review

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// branchDir writes a review directory holding two saved patches, as a review run leaves it, and returns the directory.
func branchDir(t *testing.T, branches, name string) string {
	t.Helper()
	dir := filepath.Join(branches, "branches", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"snapshot.patch", "review.patch"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRemoveBranchDeletesOnlyItsDirectory(t *testing.T) {
	branches := t.TempDir()
	dir := branchDir(t, branches, "feat%2Fexport")
	sibling := branchDir(t, branches, "feat%2Fexport-v2")
	removed, err := RemoveBranch(dir)
	if err != nil || !removed {
		t.Fatalf("RemoveBranch = %v, %v; want removed", removed, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the branch directory survived: %v", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("a sibling branch's directory was touched: %v", err)
	}
}

func TestRemoveBranchWithNoDirectoryIsDone(t *testing.T) {
	removed, err := RemoveBranch(filepath.Join(t.TempDir(), "branches", "never-reviewed"))
	if err != nil || removed {
		t.Fatalf("RemoveBranch = %v, %v; want nothing removed and no error", removed, err)
	}
}

// worktree.txt is review's record of an executor worktree it could not retire; deleting it would orphan that worktree and its saved patch. A record whose directory is gone holds nothing.
func TestRemoveBranchRefusesARecordedWorktree(t *testing.T) {
	dir := branchDir(t, t.TempDir(), "feat")
	tree := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "worktree.txt"), []byte(tree+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveBranch(dir)
	if !errors.Is(err, ErrHeld) || removed {
		t.Fatalf("RemoveBranch = %v, %v; want ErrHeld and nothing removed", removed, err)
	}
	for _, f := range []string{"snapshot.patch", "review.patch", "worktree.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s was deleted by a refused removal: %v", f, err)
		}
	}
	if err := os.Remove(tree); err != nil {
		t.Fatal(err)
	}
	if removed, err := RemoveBranch(dir); err != nil || !removed {
		t.Fatalf("RemoveBranch with the recorded worktree gone = %v, %v; want it removed", removed, err)
	}
}

// A dot segment is the one name path-escaping leaves alone, and it would point the review directory, and a removal, at a parent directory.
func TestDirRejectsDotSegments(t *testing.T) {
	for _, branch := range []string{".", ".."} {
		if _, err := Dir(t.TempDir(), branch); !errors.Is(err, ErrInvalid) {
			t.Errorf("Dir(%q) = %v, want ErrInvalid", branch, err)
		}
	}
}

func TestRemoveBranchRefusesOutsideTheBranchesDirectory(t *testing.T) {
	root := t.TempDir()
	dir := branchDir(t, root, "feat")
	// The path a ".." branch would have resolved to: the directory holding every branch.
	removed, err := RemoveBranch(filepath.Join(root, "branches"))
	if !errors.Is(err, ErrInvalid) || removed {
		t.Fatalf("RemoveBranch = %v, %v; want ErrInvalid and nothing removed", removed, err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("a branch's directory was deleted: %v", err)
	}
}

// Prune keeps every branch that still exists and every directory that records a review worktree still on disk, and a dry run deletes nothing.
func TestPruneRemovesOnlyGoneBranches(t *testing.T) {
	root := t.TempDir()
	live := (branchDir(t, root, "feat%2Flive"))
	gone := (branchDir(t, root, "feat%2Fgone"))
	held := (branchDir(t, root, "held"))
	if err := os.WriteFile(filepath.Join(held, "worktree.txt"), []byte(t.TempDir()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A record whose worktree directory is gone, as after a retirement by path, is no hold.
	stale := (branchDir(t, root, "stale"))
	if err := os.WriteFile(filepath.Join(stale, "worktree.txt"), []byte(filepath.Join(root, "retired")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	isLive := func(b string) bool { return b == "feat/live" }
	dir := filepath.Join(root, "branches")

	removed, kept, err := Prune(dir, isLive, true)
	if err != nil || len(removed) != 2 || removed[0] != gone || removed[1] != stale || len(kept) != 1 || kept[0] != held {
		t.Fatalf("dry run = %v, %v, %v; want [%s %s], [%s]", removed, kept, err, gone, stale, held)
	}
	if _, err := os.Stat(gone); err != nil {
		t.Fatalf("a dry run deleted %s: %v", gone, err)
	}

	if removed, _, err := Prune(dir, isLive, false); err != nil || len(removed) != 2 {
		t.Fatalf("Prune = %v, %v; want two directories removed", removed, err)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Errorf("a gone branch's directory survived: %v", err)
	}
	for _, d := range []string{live, held} {
		if _, err := os.Stat(filepath.Join(d, "review.patch")); err != nil {
			t.Errorf("%s was touched: %v", d, err)
		}
	}
	if removed, held, err := Prune(filepath.Join(root, "missing"), isLive, false); err != nil || removed != nil || held != nil {
		t.Errorf("Prune with no branches directory = %v, %v, %v; want nothing", removed, held, err)
	}
}
