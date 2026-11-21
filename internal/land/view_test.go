package land

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintActions(t *testing.T) {
	for _, title := range []string{"land sync", "land finish"} {
		t.Run(title, func(t *testing.T) {
			var out bytes.Buffer
			printActions(&out, title, []Action{{Done: true, Text: "updated main"}, {Text: "branch: still in use"}})
			for _, want := range []string{title, "ok updated main", "! kept branch: still in use"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output %q lacks %q", out.String(), want)
				}
			}
			if strings.Contains(out.String(), "\x1b") {
				t.Errorf("plain output contains escape codes: %q", out.String())
			}
		})
	}
}
