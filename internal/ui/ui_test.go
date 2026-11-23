package ui

import (
	"strings"
	"testing"
)

func TestPlainWritesNoEscapes(t *testing.T) {
	out := Plain.Title("t", "c") + Plain.Pill(0, "x", 4) + Plain.Warn("w") + Plain.Fail("f") + Plain.OK("o") + strings.Join(Plain.Fields([2]string{"k", "v"}), "")
	if strings.Contains(out, "\x1b") {
		t.Errorf("plain style wrote an escape: %q", out)
	}
}
