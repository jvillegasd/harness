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

func TestRunVerdictBodyRoundTrips(t *testing.T) {
	for _, result := range Results {
		v := Verdict{Run: true, Result: result, SHA: headSHA}
		body := Body(v, "- login: pass, evidence at shots/login.png\n")
		got, ok := Parse(body)
		if !ok || got != v {
			t.Errorf("Parse(Body(%+v)) = %+v, %v", v, got, ok)
		}
		if !strings.Contains(body, "## Run-as-user verdict: "+result) || !strings.Contains(body, "shots/login.png") {
			t.Errorf("body lacks its heading or notes:\n%s", body)
		}
	}
	run := Body(Verdict{Run: true, Result: "PASS", SHA: headSHA}, "")
	if v, ok := Parse(strings.Replace(run, "harness-run", "harness-review", 1)); ok {
		t.Errorf("a run marker renamed to a review's parsed as %+v", v)
	}
	if v, ok := Parse("> " + run); ok {
		t.Errorf("a quoted run verdict parsed as %+v", v)
	}
}

func TestVerdictValidate(t *testing.T) {
	ok := Verdict{Result: "PASS", Base: baseSHA, SHA: headSHA, Level: "medium"}
	if err := ok.Validate(""); err != nil {
		t.Errorf("a PASS needs no notes: %v", err)
	}
	if err := (Verdict{Run: true, Result: "PASS", SHA: headSHA}).Validate(""); err != nil {
		t.Errorf("a run PASS needs only its commit: %v", err)
	}
	for name, c := range map[string]struct {
		v     Verdict
		notes string
	}{
		"unknown result":      {Verdict{Result: "OK", Base: baseSHA, SHA: headSHA, Level: "medium"}, "n"},
		"off level":           {Verdict{Result: "PASS", Base: baseSHA, SHA: headSHA, Level: "off"}, ""},
		"short sha":           {Verdict{Result: "PASS", Base: baseSHA, SHA: "bbbb", Level: "medium"}, ""},
		"FAIL without notes":  {Verdict{Result: "FAIL", Base: baseSHA, SHA: headSHA, Level: "high"}, " \n"},
		"notes without text":  {Verdict{Result: "PASS+NOTES", Base: baseSHA, SHA: headSHA, Level: "high"}, ""},
		"review without base": {Verdict{Result: "PASS", SHA: headSHA, Level: "medium"}, ""},
		"run with a level":    {Verdict{Run: true, Result: "PASS", SHA: headSHA, Level: "medium"}, ""},
		"run with a base":     {Verdict{Run: true, Result: "PASS", Base: baseSHA, SHA: headSHA}, ""},
		"run short sha":       {Verdict{Run: true, Result: "PASS", SHA: "bbbb"}, ""},
		"run FAIL, no notes":  {Verdict{Run: true, Result: "FAIL", SHA: headSHA}, ""},
	} {
		if err := c.v.Validate(c.notes); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: Validate = %v, want ErrInvalid", name, err)
		}
	}
}
