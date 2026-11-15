package attempts

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func invoke(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Command(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func addArgs(path, before, after string, extra ...string) []string {
	args := []string{"add", "--log", path, "--hypothesis", "hypothesis", "--change", "change", "--before", before, "--after", after, "--gate", "pass", "--outcome", "kept"}
	return append(args, extra...)
}

func readTSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma, r.FieldsPerRecord = '\t', -1
	rows, err := r.ReadAll()
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	return rows
}

func TestAddCreatesHeaderAndAppendsNextID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.tsv")
	if code, _, errText := invoke(addArgs(path, "200", "150")...); code != 0 {
		t.Fatalf("first add exited %d: %s", code, errText)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if code, _, errText := invoke(addArgs(path, "150", "140")...); code != 0 {
		t.Fatalf("second add exited %d: %s", code, errText)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, first) {
		t.Fatal("second add changed the first row")
	}
	rows := readTSV(t, path)
	if len(rows) != 3 || strings.Join(rows[0], ",") != "id,time,hypothesis,change,before,after,delta,percent,gate,outcome,note" || rows[1][0] != "1" || rows[2][0] != "2" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

func TestAddComputesDeltaAndPercent(t *testing.T) {
	for _, tc := range []struct{ before, after, delta, percent string }{{"200", "150", "-50", "-25"}, {"0", "6", "6", ""}} {
		path := filepath.Join(t.TempDir(), "attempts.tsv")
		if code, _, errText := invoke(addArgs(path, tc.before, tc.after)...); code != 0 {
			t.Fatalf("add exited %d: %s", code, errText)
		}
		row := readTSV(t, path)[1]
		if row[6] != tc.delta || row[7] != tc.percent {
			t.Fatalf("delta/percent = %q/%q, want %q/%q", row[6], row[7], tc.delta, tc.percent)
		}
	}
}

func TestAddRoundsPercentAndStampsUTC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.tsv")
	if code, _, errText := invoke(addArgs(path, "3", "2")...); code != 0 {
		t.Fatalf("add exited %d: %s", code, errText)
	}
	row := readTSV(t, path)[1]
	if row[7] != "-33.33" {
		t.Fatalf("percent = %q, want -33.33", row[7])
	}
	if timestamp, err := time.Parse(time.RFC3339, row[1]); err != nil || timestamp.Location() != time.UTC {
		t.Fatalf("time %q is not RFC 3339 UTC: %v", row[1], err)
	}
}

func TestAddSanitizesTextCells(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.tsv")
	args := addArgs(path, "1", "2", "--hypothesis", "=SUM(1,2)\tmore\rnext\nline", "--note", "@note")
	if code, _, errText := invoke(args...); code != 0 {
		t.Fatalf("add exited %d: %s", code, errText)
	}
	row := readTSV(t, path)[1]
	if row[2] != "'=SUM(1,2) more next line" || row[10] != "'@note" {
		t.Fatalf("text cells not neutralized: %#v", row)
	}
	for _, prefix := range []string{"=", "+", "-", "@"} {
		path := filepath.Join(t.TempDir(), "formula.tsv")
		args := addArgs(path, "1", "2", "--change", prefix+"formula")
		if code, _, _ := invoke(args...); code != 0 || readTSV(t, path)[1][3] != "'"+prefix+"formula" {
			t.Fatalf("prefix %q was not stored as text", prefix)
		}
	}
}

func TestRejectedAddLeavesLogUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.tsv")
	if code, _, _ := invoke(addArgs(path, "1", "2")...); code != 0 {
		t.Fatal("setup add failed")
	}
	before, _ := os.ReadFile(path)
	cases := [][]string{
		addArgs(path, "1", "2", "--gate", "maybe"),
		addArgs(path, "1", "2", "--outcome", "lost"),
		addArgs(path, "x", "2"),
		addArgs(path, "1", "x"),
		{"add", "--log", path},
	}
	for _, args := range cases {
		if code, _, _ := invoke(args...); code != 2 {
			t.Errorf("%v exited %d, want 2", args, code)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Errorf("%v changed the log", args)
		}
	}
}

func TestAddRefusesLogInsideCheckout(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	path := filepath.Join(root, "nested", "attempts.tsv")
	if err := os.Mkdir(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{path, filepath.Join(root, "link", "nested", "attempts.tsv")} {
		if code, _, _ := invoke(addArgs(target, "1", "2")...); code != 2 {
			t.Errorf("inside path %s exited %d", target, code)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("inside path %s was written: %v", target, err)
		}
	}
}

func TestListJSONUsesTypedNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempts.tsv")
	if code, _, _ := invoke(addArgs(path, "200", "150")...); code != 0 {
		t.Fatal("setup add failed")
	}
	code, out, errText := invoke("list", "--log", path, "--json")
	if code != 0 {
		t.Fatalf("list exited %d: %s", code, errText)
	}
	if !strings.Contains(out, `"id":1`) || !strings.Contains(out, `"before":200`) || !strings.Contains(out, `"after":150`) || !strings.Contains(out, `"delta":-50`) || !strings.Contains(out, `"percent":-25`) {
		t.Fatalf("JSON does not contain typed numbers: %s", out)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "before", "after", "delta", "percent"} {
		if _, ok := decoded[0][key].(float64); !ok {
			t.Errorf("%s is not a JSON number: %#v", key, decoded[0][key])
		}
	}
	code, raw, errText := invoke("list", "--log", path)
	stored, _ := os.ReadFile(path)
	if code != 0 || errText != "" || raw != string(stored) {
		t.Fatalf("list did not print stored TSV: code=%d stdout=%q stderr=%q", code, raw, errText)
	}
}
