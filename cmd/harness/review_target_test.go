package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestReviewTargetCommand proves the CLI is wired to target resolution: a pull request resolves through gh to its head and key, and an unknown branch is refused with nothing on stdout.
func TestReviewTargetCommand(t *testing.T) {
	_, head, run := prRepo(t)
	forge := `"pr view") echo '{"headRefName":"` + branch + `","headRefOid":"` + head + `","baseRefName":"main","isCrossRepository":false}';;
`
	r := run(forge, "review", "target", "7")
	var got struct {
		Kind, Base, SHA, Key string
		Foreign, Fork        bool
	}
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &got) != nil || got.Kind != "pr" || got.SHA != head || got.Key != branch || got.Foreign {
		t.Fatalf("target 7 exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
	if r := run("", "review", "target", "nope"); r.code != 2 || r.stdout != "" || !strings.Contains(r.stderr, "nope") {
		t.Errorf("an unknown branch exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
}
