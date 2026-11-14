package reviewledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func snapshotGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(bytes.TrimSpace(out))
}

func snapshotRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	snapshotGit(t, root, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "owned.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshotGit(t, root, "add", "owned.txt")
	snapshotGit(t, root, "commit", "-q", "-m", "base")
	return root, snapshotGit(t, root, "rev-parse", "HEAD")
}

func TestSnapshotContentIdentityAndOwnership(t *testing.T) {
	root, base := snapshotRepo(t)
	if err := os.WriteFile(filepath.Join(root, "owned.txt"), []byte("changed\x00bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.bin"), []byte{0, 1, 255}, 0o644); err != nil {
		t.Fatal(err)
	}
	firstPath, firstDigest, err := Snapshot(root, base, "work", []string{"owned.txt", "new.bin"})
	if err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(patch)
	if firstDigest != hex.EncodeToString(hash[:]) {
		t.Fatalf("digest %q does not hash patch", firstDigest)
	}
	clean := t.TempDir()
	snapshotGit(t, clean, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(clean, "owned.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshotGit(t, clean, "add", "owned.txt")
	snapshotGit(t, clean, "commit", "-q", "-m", "base")
	if _, err := exec.Command("git", "-C", clean, "apply", firstPath).CombinedOutput(); err != nil {
		t.Fatalf("apply saved patch: %v", err)
	}
	for name, want := range map[string][]byte{"owned.txt": []byte("changed\x00bytes"), "new.bin": {0, 1, 255}} {
		got, err := os.ReadFile(filepath.Join(clean, name))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("applied %s = %v, %v; want %v", name, got, err, want)
		}
	}
	secondPath, secondDigest, err := Snapshot(root, base, "work", []string{"owned.txt", "new.bin"})
	if err != nil || secondDigest != firstDigest || secondPath != firstPath {
		t.Fatalf("repeat snapshot = %q, %q, %v; want same path and digest", secondPath, secondDigest, err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, outsideDigest, err := Snapshot(root, base, "work", []string{"owned.txt", "new.bin"})
	if err != nil || outsideDigest != firstDigest {
		t.Fatalf("outside change affected snapshot: %q, %v", outsideDigest, err)
	}
	if err := os.WriteFile(filepath.Join(root, "owned.txt"), []byte("changed\x00byteS"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, changedDigest, err := Snapshot(root, base, "work", []string{"owned.txt", "new.bin"})
	if err != nil || changedDigest == firstDigest {
		t.Fatalf("owned byte change did not change digest: %q, %v", changedDigest, err)
	}
}

func TestSnapshotPreservesIndexAndRejectsEmptyOrBadBase(t *testing.T) {
	root, base := snapshotRepo(t)
	if err := os.WriteFile(filepath.Join(root, "owned.txt"), []byte("part staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshotGit(t, root, "add", "owned.txt")
	if err := os.WriteFile(filepath.Join(root, "owned.txt"), []byte("staged and unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexBefore, _ := os.ReadFile(filepath.Join(root, ".git", "index"))
	statusBefore := snapshotGit(t, root, "status", "--porcelain")
	stagedBefore := snapshotGit(t, root, "diff", "--cached", "--binary")
	if _, _, err := Snapshot(root, base, "work", []string{"owned.txt"}); err != nil {
		t.Fatal(err)
	}
	indexAfter, _ := os.ReadFile(filepath.Join(root, ".git", "index"))
	if string(indexBefore) != string(indexAfter) || snapshotGit(t, root, "status", "--porcelain") != statusBefore || snapshotGit(t, root, "diff", "--cached", "--binary") != stagedBefore {
		t.Fatal("snapshot changed the index or git status")
	}
	if err := os.WriteFile(filepath.Join(root, "owned.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branchDir := filepath.Dir(mustFile(t, root, "work"))
	beforeEntries, err := os.ReadDir(branchDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Snapshot(root, base, "work", []string{"owned.txt"}); err == nil {
		t.Fatal("empty snapshot succeeded")
	}
	afterEntries, err := os.ReadDir(branchDir)
	if err != nil || !reflect.DeepEqual(beforeEntries, afterEntries) {
		t.Fatalf("empty snapshot changed files: before %v, after %v, error %v", beforeEntries, afterEntries, err)
	}
	if _, _, err := Snapshot(root, "missing-base", "work", []string{"owned.txt"}); err == nil {
		t.Fatal("unresolvable base succeeded")
	}
}

func mustFile(t *testing.T, root, key string) string {
	t.Helper()
	file, err := File(root, key)
	if err != nil {
		t.Fatal(err)
	}
	return file
}
