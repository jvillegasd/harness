package update

import (
	"bytes"
	"strings"
	"testing"
)

func TestSummaryPlain(t *testing.T) {
	var out bytes.Buffer
	r := runner{out: &out, results: []string{"skipped: example: not installed"}}
	r.summary()
	if got := out.String(); strings.Contains(got, "\x1b") || !strings.Contains(got, "harness update") || !strings.Contains(got, "example: not installed") {
		t.Fatalf("summary: %q", got)
	}
}
