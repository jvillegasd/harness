package cli

import (
	"flag"
	"io"
	"slices"
	"testing"
)

func TestDashDashIsAFlagValueOrTheEndOfFlags(t *testing.T) {
	for _, c := range []struct {
		args       []string
		body       string
		positional []string
	}{
		{[]string{"--body-file", "--", "42", "--head", "abc"}, "--", []string{"42"}},
		{[]string{"--merge", "--", "--head"}, "", []string{"--head"}},
	} {
		flags := flag.NewFlagSet("merge", flag.ContinueOnError)
		body := flags.String("body-file", "", "")
		flags.String("head", "", "")
		flags.Bool("merge", false, "")
		positional, code := Parse("harness land", flags, c.args, "", io.Discard, io.Discard)
		if code != -1 || *body != c.body || !slices.Equal(positional, c.positional) {
			t.Errorf("%q: code %d, body %q, positional %q; want body %q, positional %q", c.args, code, *body, positional, c.body, c.positional)
		}
	}
}
