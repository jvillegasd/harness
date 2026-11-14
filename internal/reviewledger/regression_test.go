package reviewledger

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAddWorktreePatchNotSkipped covers a second add with a patch over an existing worktree: it refuses instead of returning a checkout without the patch.
func TestAddWorktreePatchNotSkipped(t *testing.T) {
	r := newCoverRepo(t)
	bad := filepath.Join(t.TempDir(), "bad.patch")
	os.WriteFile(bad, []byte("not a patch\n"), 0o644)
	if _, err := AddWorktree(r.dir, "feat", "HEAD", bad); err == nil {
		t.Fatal("a bad patch applied")
	}
	os.WriteFile(filepath.Join(r.dir, "a.go"), []byte("patched\n"), 0o644)
	good := filepath.Join(t.TempDir(), "good.patch")
	os.WriteFile(good, []byte(r.git("diff", "--binary")+"\n"), 0o644)
	r.git("checkout", "--", "a.go")
	path, err := AddWorktree(r.dir, "feat", "HEAD", good)
	if err == nil {
		t.Errorf("add returned %s without applying the patch", path)
	}
	if _, err := RetireWorktree(r.dir, "feat", false); err != nil {
		t.Fatal(err)
	}
	if path, err = AddWorktree(r.dir, "feat", "HEAD", good); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(path, "a.go")); string(raw) != "patched\n" {
		t.Errorf("a fresh worktree holds %q", raw)
	}
}
