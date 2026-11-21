package install

import (
	"bytes"
	"strings"
	"testing"
)

func TestReportsPlain(t *testing.T) {
	for _, verb := range []string{"install", "uninstall"} {
		var out bytes.Buffer
		in := installer{verb: verb, stdout: &out}
		in.log("Installed %s", "example")
		if got := out.String(); strings.Contains(got, "\x1b") || !strings.Contains(got, "ok Installed example") {
			t.Fatalf("%s report: %q", verb, got)
		}
	}
}
