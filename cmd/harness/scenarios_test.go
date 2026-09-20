package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	attemptPath           = regexp.MustCompile(` -> .*`)
	evidenceDigestPattern = regexp.MustCompile(`bind evidence_digest [0-9a-f]{64}`)
)

// normalizeReport replaces the attempt path and the attempt-dependent evidence digest, so the golden asserts the reason text but not the digest value, which the unit test asserts.
func normalizeReport(text string) string {
	return evidenceDigestPattern.ReplaceAllString(attemptPath.ReplaceAllString(text, " -> <attempt>"), "bind evidence_digest <digest>")
}

// TestScenariosValidateAndRunAgainstRecordedFixtures drives the command boundary over the shipped matrix without paid sessions, comparing each result to the golden captured from the Python runner before it was deleted.
func TestScenariosValidateAndRunAgainstRecordedFixtures(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()

	validate := runBinary(t, root, "scenarios", "validate", "--out", out)
	if validate.code != 0 {
		t.Fatalf("validate exited %d: %s", validate.code, validate.stderr)
	}
	assertGolden(t, "validate.golden", validate.stdout)

	run := runBinary(t, root, "scenarios", "run", "--out", out)
	assertGolden(t, "run.golden", normalizeReport(run.stdout))

	report := runBinary(t, root, "scenarios", "report", "--out", out)
	if report.code != 0 {
		t.Fatalf("report exited %d: %s", report.code, report.stderr)
	}
	assertGolden(t, "report.golden", normalizeReport(report.stdout))
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", "scenarios", name))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("%s differs from the captured golden:\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

// TestScenariosResolveTheCheckoutFromANestedDirectory runs from a nested directory, where the matrix default has to resolve the enclosing checkout rather than the invocation directory.
func TestScenariosResolveTheCheckoutFromANestedDirectory(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "internal", "scenarios")
	r := runBinary(t, nested, "scenarios", "validate", "--out", t.TempDir())
	if r.code != 0 || !strings.Contains(r.stdout, "local-tooling: pass") {
		t.Fatalf("scenarios from a nested directory exited %d with stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}

func TestScenariosValidateRejectsABadMatrix(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	matrix := filepath.Join(t.TempDir(), "matrix.json")
	if err := os.WriteFile(matrix, []byte(`{"version":1,"scenarios":[{"id":"s","command":"c","prompt":"p","artifacts":{}}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runBinary(t, root, "scenarios", "validate", "--matrix", matrix, "--out", t.TempDir())
	if r.code != 1 || !strings.Contains(r.stderr, "artifacts") {
		t.Fatalf("validate exited %d with stderr %q", r.code, r.stderr)
	}
}
