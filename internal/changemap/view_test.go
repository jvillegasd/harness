package changemap

import (
	"strings"
	"testing"
)

func TestDrawPlainReport(t *testing.T) {
	var out strings.Builder
	Report{Base: "main", Files: []File{{Path: "file.go", Tier: Structural, Reason: "source"}}, Totals: map[Tier]int{Structural: 1}}.draw(&out)
	for _, want := range []string{"harness change-map", "main", "file.go", "structural", "source"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q: %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "\x1b") {
		t.Fatal("plain output contains escape codes")
	}
}
