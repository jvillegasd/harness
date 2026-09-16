package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jvillegasd/harness/internal/checkout"
)

var split = time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)

// TestReportForRendersForTheGivenCheckout pins that the shared reporting
// operation reports for the checkout it is handed, with nothing to discover.
func TestReportForRendersForTheGivenCheckout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	out, err := ReportFor(checkout.Checkout{Root: t.TempDir()}, split)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "split 2026-09-15") || !strings.Contains(out, "no sessions found") {
		t.Fatalf("report = %q", out)
	}
}

// TestReportForKeepsTheReportWhenCohortsFail pins that a broken cohort file
// warns and still yields the report, exactly as the command does.
func TestReportForKeepsTheReportWhenCohortsFail(t *testing.T) {
	root := t.TempDir()
	bench := filepath.Join(root, "bench")
	if err := os.MkdirAll(bench, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bench, "cohorts.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	out, err := ReportFor(checkout.Checkout{Root: root}, split)
	if err != nil {
		t.Fatalf("a cohort warning failed the report: %v", err)
	}
	if !strings.Contains(out, "cohorts.json") || !strings.Contains(out, "split 2026-09-15") {
		t.Fatalf("report did not carry the warning and the report: %q", out)
	}
}
