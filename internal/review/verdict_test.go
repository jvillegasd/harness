package review

import (
	"errors"
	"strings"
	"testing"
)

var (
	baseSHA = strings.Repeat("a", 40)
	headSHA = strings.Repeat("b", 40)
)

// A posted verdict reads back as the same verdict, whatever notes follow the marker.
func TestVerdictBodyRoundTrips(t *testing.T) {
	for _, result := range Results {
		v := Verdict{Result: result, Base: baseSHA, SHA: headSHA, Level: "high"}
		body := Body(v, "- **bug** `a.go:4`: wrong sign\n  Proved by: go test ./a\n")
		got, ok := Parse(body)
		if !ok || got != v {
			t.Errorf("Parse(Body(%+v)) = %+v, %v", v, got, ok)
		}
		if !strings.Contains(body, "## Review verdict: "+result) || !strings.Contains(body, "wrong sign") {
			t.Errorf("body lacks its heading or notes:\n%s", body)
		}
	}
}

// Only a comment that starts with the marker is a verdict, so quoting one in another comment, or a malformed marker, forges nothing.
func TestParseRefusesAnythingButALeadingMarker(t *testing.T) {
	good := Body(Verdict{Result: "PASS", Base: baseSHA, SHA: headSHA, Level: "medium"}, "")
	for name, body := range map[string]string{
		"quoted":       "> " + good,
		"after text":   "Looks fine.\n" + good,
		"short sha":    strings.Replace(good, headSHA, headSHA[:12], 1),
		"other result": strings.Replace(good, "verdict=PASS", "verdict=OK", 1),
		"no marker":    "## Review verdict: PASS\n",
	} {
		if v, ok := Parse(body); ok {
			t.Errorf("%s: Parse = %+v, want no verdict", name, v)
		}
	}
	if _, ok := Parse(strings.Replace(good, "-->\n", "-->\r\n", 1)); !ok {
		t.Error("a CRLF marker line must still parse")
	}
}

func TestVerdictValidate(t *testing.T) {
	ok := Verdict{Result: "PASS", Base: baseSHA, SHA: headSHA, Level: "medium"}
	if err := ok.Validate(""); err != nil {
		t.Errorf("a PASS needs no notes: %v", err)
	}
	for name, c := range map[string]struct {
		v     Verdict
		notes string
	}{
		"unknown result":     {Verdict{Result: "OK", Base: baseSHA, SHA: headSHA, Level: "medium"}, "n"},
		"off level":          {Verdict{Result: "PASS", Base: baseSHA, SHA: headSHA, Level: "off"}, ""},
		"short sha":          {Verdict{Result: "PASS", Base: baseSHA, SHA: "bbbb", Level: "medium"}, ""},
		"FAIL without notes": {Verdict{Result: "FAIL", Base: baseSHA, SHA: headSHA, Level: "high"}, " \n"},
		"notes without text": {Verdict{Result: "PASS+NOTES", Base: baseSHA, SHA: headSHA, Level: "high"}, ""},
	} {
		if err := c.v.Validate(c.notes); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: Validate = %v, want ErrInvalid", name, err)
		}
	}
}
