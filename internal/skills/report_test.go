package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/checkout"
)

func TestVerifyPlain(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills-lock.json"), []byte(`{"skills":{"example":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := verify(checkout.Checkout{Root: root}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "\x1b") || !strings.Contains(got, "harness skills verify") || !strings.Contains(got, "example: unverifiable") {
		t.Fatalf("report: %q", got)
	}
}
