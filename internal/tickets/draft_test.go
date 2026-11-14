package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// draft renders a well-formed draft blocked by the given text.
func draft(title, blocked string) string {
	return "# " + title + "\n\n## What to build\n\nA thing.\n\n## Acceptance criteria\n\n- It works.\n\n## Seam\n\nThe package tests.\n\n## Blocked by\n\n" + blocked + "\n\n## Out of scope\n\nNothing.\n"
}

// feature writes the named files under root/rel and returns rel.
func feature(t *testing.T, root string, files map[string]string) string {
	t.Helper()
	rel := filepath.Join("docs", "agents", "tickets", "demo")
	for name, body := range files {
		path := filepath.Join(root, rel, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return rel
}

func messages(problems []Problem) string {
	var lines []string
	for _, p := range problems {
		lines = append(lines, p.String())
	}
	return strings.Join(lines, "\n")
}

func TestCheckPassesWellFormedDrafts(t *testing.T) {
	root := t.TempDir()
	rel := feature(t, root, map[string]string{
		"01-first.md":  draft("01 First", "none"),
		"02-second.md": draft("02 Second", "01"),
		"03-third.md":  draft("03 Third", "#1, 02"),
	})
	ts, problems, err := Check(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems on well-formed drafts:\n%s", messages(problems))
	}
	if len(ts) != 3 || ts[2].Number != 3 || len(ts[2].BlockedBy) != 2 || ts[2].BlockedBy[0] != 1 || ts[2].BlockedBy[1] != 2 {
		t.Fatalf("tickets = %+v", ts)
	}
}

func TestCheckReportsMissingAndEmptySections(t *testing.T) {
	root := t.TempDir()
	missing := strings.Replace(draft("01 First", "none"), "## Seam\n\nThe package tests.\n\n", "", 1)
	empty := strings.Replace(draft("02 Second", "none"), "- It works.\n", "  \n", 1)
	rel := feature(t, root, map[string]string{"01-first.md": missing, "02-second.md": empty})
	_, problems, err := Check(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	got := messages(problems)
	for _, want := range []string{
		filepath.Join(rel, "01-first.md") + ":1: missing section Seam",
		filepath.Join(rel, "02-second.md") + ":7: empty section Acceptance criteria",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
	if len(problems) != 2 {
		t.Errorf("want 2 problems, got:\n%s", got)
	}
}

func TestCheckReportsBothDraftsSharingANumber(t *testing.T) {
	root := t.TempDir()
	rel := feature(t, root, map[string]string{"01-a.md": draft("01 A", "none"), "01-b.md": draft("01 B", "none")})
	_, problems, _ := Check(root, rel)
	got := messages(problems)
	for _, name := range []string{"01-a.md", "01-b.md"} {
		if !strings.Contains(got, filepath.Join(rel, name)+":1: ticket number 01 is shared") {
			t.Errorf("%s not reported:\n%s", name, got)
		}
	}
}

func TestCheckReportsMissingBlockerAndBadBlockedBy(t *testing.T) {
	root := t.TempDir()
	rel := feature(t, root, map[string]string{
		"01-a.md": draft("01 A", "07"),
		"02-b.md": draft("02 B", "the first one"),
	})
	_, problems, _ := Check(root, rel)
	got := messages(problems)
	if !strings.Contains(got, filepath.Join(rel, "01-a.md")+":17: blocked by 07, which is not a ticket of this feature") {
		t.Errorf("missing blocker not reported:\n%s", got)
	}
	if !strings.Contains(got, filepath.Join(rel, "02-b.md")+":17: Blocked by must be none or ticket numbers") {
		t.Errorf("bad Blocked by not reported:\n%s", got)
	}
}

func TestCheckReportsACycleOnce(t *testing.T) {
	root := t.TempDir()
	rel := feature(t, root, map[string]string{
		"01-a.md": draft("01 A", "03"),
		"02-b.md": draft("02 B", "01"),
		"03-c.md": draft("03 C", "02"),
		"04-d.md": draft("04 D", "01"),
	})
	_, problems, _ := Check(root, rel)
	if len(problems) != 1 || problems[0].Message != "blocking cycle among 01, 02, 03" || problems[0].Path != filepath.Join(rel, "01-a.md") {
		t.Fatalf("want one cycle problem, got:\n%s", messages(problems))
	}
}

func TestCheckReportsAFileThatIsNotADraft(t *testing.T) {
	root := t.TempDir()
	rel := feature(t, root, map[string]string{"01-a.md": draft("01 A", "none"), "notes.md": "scratch\n", "1-short.md": draft("1 Short", "none")})
	_, problems, _ := Check(root, rel)
	got := messages(problems)
	for _, name := range []string{"notes.md", "1-short.md"} {
		if !strings.Contains(got, filepath.Join(rel, name)+":1: not a draft; name it <NN>-<slug>.md") {
			t.Errorf("%s not reported:\n%s", name, got)
		}
	}
}

func TestCheckReportsAMissingTitle(t *testing.T) {
	root := t.TempDir()
	rel := feature(t, root, map[string]string{"01-a.md": strings.TrimPrefix(draft("01 A", "none"), "# 01 A\n")})
	_, problems, _ := Check(root, rel)
	if got := messages(problems); got != filepath.Join(rel, "01-a.md")+":1: missing title; start with # <NN> <title>" {
		t.Fatalf("got:\n%s", got)
	}
}

func TestFeaturesListsEachDraftDirectory(t *testing.T) {
	root := t.TempDir()
	if got, err := Features(root); err != nil || len(got) != 0 {
		t.Fatalf("no draft directory: %v, %v", got, err)
	}
	feature(t, root, map[string]string{"01-a.md": draft("01 A", "none")})
	if err := os.MkdirAll(filepath.Join(root, DraftDir, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Features(root)
	if err != nil || strings.Join(got, ",") != "demo,other" {
		t.Fatalf("Features = %v, %v", got, err)
	}
}
