package ui

import (
	"strings"
	"testing"
)

func TestTableAlignsStyledCellsByWhatShows(t *testing.T) {
	for _, s := range []Style{Plain, Colored} {
		lines := s.Table([]string{"name", "state"}, [][]string{{s.Bold("a"), s.Good("landed")}, {"longer", s.Bad("held")}})
		var stripped []string
		for _, l := range lines {
			stripped = append(stripped, strip(l))
		}
		want := []string{"NAME    STATE", "a       landed", "longer  held"}
		if strings.Join(stripped, "\n") != strings.Join(want, "\n") {
			t.Errorf("table (color %v):\n%s", s.color, strings.Join(stripped, "\n"))
		}
	}
}

func TestPlainWritesNoEscapes(t *testing.T) {
	out := Plain.Title("t", "c") + Plain.Pill(0, "x", 4) + Plain.Warn("w") + Plain.Fail("f") + Plain.OK("o") + strings.Join(Plain.Fields([2]string{"k", "v"}), "")
	if strings.Contains(out, "\x1b") {
		t.Errorf("plain style wrote an escape: %q", out)
	}
}

func strip(s string) string {
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			return s
		}
		j := strings.IndexByte(s[i:], 'm')
		s = s[:i] + s[i+j+1:]
	}
}
