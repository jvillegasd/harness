package tickets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type fakeForge struct {
	issues     map[int]*fakeIssue
	next       int
	calls      []string
	failCreate int
	creates    int
	retitle    int
}

type fakeIssue struct {
	Title, Body string
	Labels      []string
	BlockedBy   []int
}

func newFake() *fakeForge { return &fakeForge{issues: map[int]*fakeIssue{}, next: 40} }

func (f *fakeForge) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	flag := func(name string) string {
		for i := range args[:len(args)-1] {
			if args[i] == name {
				return args[i+1]
			}
		}
		return ""
	}
	switch {
	case args[0] == "issue" && args[1] == "create":
		f.creates++
		if f.creates == f.failCreate {
			return nil, errors.New("HTTP 502")
		}
		f.next++
		f.issues[f.next] = &fakeIssue{Title: flag("--title"), Body: flag("--body")}
		if f.next == f.retitle {
			f.issues[f.next].Title = "edited on the forge"
		}
		return []byte(fmt.Sprintf("https://github.com/o/r/issues/%d\n", f.next)), nil
	case args[0] == "issue" && args[1] == "view":
		n, _ := strconv.Atoi(args[2])
		i := f.issues[n]
		if i == nil {
			return nil, errors.New("not found")
		}
		return json.Marshal(map[string]any{"title": i.Title, "body": i.Body})
	case args[0] == "api" && args[1] == "-X":
		n := f.number(args[3])
		id, _ := strconv.Atoi(strings.TrimPrefix(args[5], "issue_id="))
		if f.issues[id-1000] == nil {
			return nil, fmt.Errorf("no issue with database id %d", id)
		}
		f.issues[n].BlockedBy = append(f.issues[n].BlockedBy, id-1000)
		return []byte("{}"), nil
	case args[0] == "api" && strings.HasSuffix(args[1], "/dependencies/blocked_by"):
		list := []map[string]int{}
		for _, b := range f.issues[f.number(args[1])].BlockedBy {
			list = append(list, map[string]int{"number": b})
		}
		return json.Marshal(list)
	case args[0] == "api":
		return []byte(fmt.Sprintf(`{"id":%d,"number":%d}`, 1000+f.number(args[1]), f.number(args[1]))), nil
	}
	return nil, fmt.Errorf("unexpected gh %s", strings.Join(args, " "))
}

func (f *fakeForge) number(path string) int {
	rest := strings.TrimPrefix(path, repoPath)
	n, _ := strconv.Atoi(strings.SplitN(rest, "/", 2)[0])
	return n
}

func (f *fakeForge) creatTitles() []string {
	var titles []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "issue create --title ") {
			titles = append(titles, strings.SplitN(strings.TrimPrefix(c, "issue create --title "), " --body", 2)[0])
		}
	}
	return titles
}

func threeDrafts(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	rel := feature(t, root, map[string]string{
		"01-a.md": draft("01 Alpha", "none"),
		"02-b.md": draft("02 Beta", "none"),
		"03-c.md": draft("03 Gamma", "01, 02"),
	})
	return root, rel
}

func checked(t *testing.T, root, rel string) []Ticket {
	t.Helper()
	ts, problems, err := Check(root, rel)
	if err != nil || len(problems) != 0 {
		t.Fatalf("drafts do not check: %v %v", err, problems)
	}
	return ts
}

func TestPublishCreatesBlockersFirstWithDatabaseIDs(t *testing.T) {
	root, rel := threeDrafts(t)
	f := newFake()
	p, err := Publish(root, checked(t, root, rel), f.run)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.creatTitles(), ","); got != "Alpha,Beta,Gamma" {
		t.Errorf("create order = %s", got)
	}
	posts := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, "api -X POST") {
			posts++
			if !strings.HasSuffix(c, "issue_id=1041") && !strings.HasSuffix(c, "issue_id=1042") {
				t.Errorf("edge not sent with a database id: %s", c)
			}
		}
	}
	if posts != 2 || fmt.Sprint(f.issues[43].BlockedBy) != "[41 42]" {
		t.Errorf("edges: %d posts, 43 blocked by %v", posts, f.issues[43].BlockedBy)
	}
	if !strings.Contains(f.issues[43].Body, "## Blocked by\n\n#41, #42\n") {
		t.Errorf("Blocked by not rewritten to issues:\n%s", f.issues[43].Body)
	}
	if len(p.Issues) != 3 || p.Issues[2] != "https://github.com/o/r/issues/43" {
		t.Errorf("issues = %v", p.Issues)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, rel)); len(entries) != 0 {
		t.Errorf("drafts remain after a full success: %v", entries)
	}
}

func TestPublishResumesAfterAFailedCreateWithoutDuplicates(t *testing.T) {
	root, rel := threeDrafts(t)
	before := map[string]string{}
	for _, name := range []string{"02-b.md", "03-c.md"} {
		raw, _ := os.ReadFile(filepath.Join(root, rel, name))
		before[name] = string(raw)
	}
	f := newFake()
	f.failCreate = 2
	p, err := Publish(root, checked(t, root, rel), f.run)
	if err == nil {
		t.Fatal("a failed create did not stop the run")
	}
	if len(p.Created) != 1 || len(p.Remains) != 2 {
		t.Errorf("report: created %v, remains %v", p.Created, p.Remains)
	}
	raw, _ := os.ReadFile(filepath.Join(root, rel, "01-a.md"))
	if !strings.Contains(string(raw), "\nIssue: https://github.com/o/r/issues/41\n") {
		t.Errorf("the first draft does not record its issue:\n%s", raw)
	}
	for name, want := range before {
		if raw, _ := os.ReadFile(filepath.Join(root, rel, name)); string(raw) != want {
			t.Errorf("%s changed after the failure", name)
		}
	}

	f.calls, f.failCreate = nil, 0
	if _, err := Publish(root, checked(t, root, rel), f.run); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.creatTitles(), ","); got != "Beta,Gamma" {
		t.Errorf("rerun created %s", got)
	}
	if len(f.issues) != 3 {
		t.Errorf("rerun left %d issues", len(f.issues))
	}
}

func TestPublishKeepsADraftWhoseIssueDoesNotMatch(t *testing.T) {
	root, rel := threeDrafts(t)
	f := newFake()
	f.retitle = 42
	p, err := Publish(root, checked(t, root, rel), f.run)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Kept) != 1 || p.Kept[0] != filepath.Join(rel, "02-b.md") || !strings.Contains(p.Mismatch[0], "edited on the forge") {
		t.Errorf("kept %v, mismatch %v", p.Kept, p.Mismatch)
	}
	if _, err := os.Stat(filepath.Join(root, rel, "02-b.md")); err != nil {
		t.Error("the mismatched draft was removed")
	}
}

func TestPlanListsEveryIssueAndEdge(t *testing.T) {
	root, rel := threeDrafts(t)
	var out strings.Builder
	Plan(checked(t, root, rel), &out)
	want := "create 01 \"Alpha\"\ncreate 02 \"Beta\"\ncreate 03 \"Gamma\"\nedge   03 blocked by 01\nedge   03 blocked by 02\n"
	if out.String() != want {
		t.Errorf("plan:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestPublishKeepsTheWholeBlockerChainOfAKeptDraft(t *testing.T) {
	root := t.TempDir()
	rel := feature(t, root, map[string]string{
		"01-a.md": draft("01 Alpha", "none"),
		"02-b.md": draft("02 Beta", "01"),
		"03-c.md": draft("03 Gamma", "02"),
	})
	f := newFake()
	f.retitle = 43
	if _, err := Publish(root, checked(t, root, rel), f.run); err != nil {
		t.Fatal(err)
	}
	if _, problems, err := Check(root, rel); err != nil || len(problems) != 0 {
		t.Fatalf("the kept drafts no longer check: %v %v", err, problems)
	}
}
