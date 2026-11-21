package review

import (
	"bytes"
	"strings"
	"testing"
)

func TestDrawScorePlain(t *testing.T) {
	var out bytes.Buffer
	drawScore(&out, Comparison{Summary: Summary{Date: "2026-01-01", BugCases: 2, Caught: 1, Recall: 0.5, FalsePositives: 3}})
	for _, want := range []string{"harness review evals score", "2026-01-01", "0.5", "false positives", "3", "no previous run"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), "\x1b") {
		t.Fatal("plain output contains escape codes")
	}
}
