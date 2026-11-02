package main

import (
	"strings"
	"testing"
)

// TestReviewLevelCommand proves the CLI is wired to the level rules, and that an install keeps the stored level. Which values are accepted is settled in internal/reviewlevel.
func TestReviewLevelCommand(t *testing.T) {
	base := modelsBase(t)
	level := func(args ...string) result {
		return runHarness(t, base, nil, append([]string{"review", "level"}, args...)...)
	}
	if r := ok(t, level("--json"), "level --json"); strings.TrimSpace(r.stdout) != `{"level":"medium","source":"default"}` {
		t.Errorf("default level printed %q", r.stdout)
	}
	ok(t, level("high"), "set high")
	before := readFile(t, stateFile(base))
	for _, bad := range [][]string{{"max"}, {"--json", "off"}, {"off", "--json"}} {
		if r := level(bad...); r.code != 2 || r.stderr == "" {
			t.Errorf("level %v exited %d with %q, want 2 and a diagnostic", bad, r.code, r.stderr)
		}
	}
	if readFile(t, stateFile(base)) != before {
		t.Error("a rejected level rewrote state.json")
	}
	ok(t, runInstaller(t, base, "--skip-packages"), "install")
	if r := ok(t, level(), "level"); strings.TrimSpace(r.stdout) != "high (global)" {
		t.Errorf("after install the level is %q, want high (global)", r.stdout)
	}
	if r := ok(t, level("--help"), "level --help"); r.stdout == "" {
		t.Error("level --help printed nothing")
	}
}
