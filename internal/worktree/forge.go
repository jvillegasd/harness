package worktree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Forge runs one gh command and returns its stdout.
type Forge func(args ...string) ([]byte, error)

// Pull is what the forge says about a worktree's branch: its pull request, and for a merged one whose branch the audit could not prove landed, the merged head to retire it into.
type Pull struct {
	Number int
	State  string // OPEN, MERGED, or CLOSED; empty when the branch has no pull request
	Into   string // the merged head, fetched, to pass as --into
	Err    error  // the forge read or the fetch failed, so the forge state is unavailable
}

// Held reports whether an open pull request holds the row.
func (p Pull) Held() bool { return p.State == "OPEN" }

// String is the audit's PR column.
func (p Pull) String() string {
	switch {
	case p.Err != nil:
		return "unavailable"
	case p.State == "":
		return "none"
	case p.Held():
		return fmt.Sprintf("#%d OPEN held", p.Number)
	}
	return fmt.Sprintf("#%d %s", p.Number, p.State)
}

// ReadPull asks the forge for the pull request of a status's branch, preferring an open one, then the newest merged one. For an unlanded branch whose pull request merged, it fetches that pull request's head from origin into dir, so retiring with --into that head proves the branch holds nothing the merged pull request lacked.
func ReadPull(dir string, s Status, forge Forge) Pull {
	if s.Branch == "" {
		return Pull{}
	}
	out, err := forge("pr", "list", "--head", s.Branch, "--state", "all", "--json", "number,state,headRefOid")
	if err != nil {
		return Pull{Err: err}
	}
	var pulls []struct {
		Number     int
		State      string
		HeadRefOid string
	}
	if err := json.Unmarshal(out, &pulls); err != nil {
		return Pull{Err: fmt.Errorf("reading pull requests for %s: %w", s.Branch, err)}
	}
	pick := -1
	for _, want := range []string{"OPEN", "MERGED", "CLOSED"} {
		for i, p := range pulls {
			if pick < 0 && p.State == want {
				pick = i
			}
		}
	}
	if pick < 0 {
		return Pull{}
	}
	p := Pull{Number: pulls[pick].Number, State: pulls[pick].State}
	if p.State == "MERGED" && s.State == "unlanded" {
		head := pulls[pick].HeadRefOid
		if _, err := git(dir, "fetch", "--quiet", "origin", "refs/pull/"+strconv.Itoa(p.Number)+"/head"); err != nil {
			p.Err = err
			return p
		}
		if _, err := git(dir, "cat-file", "-e", head+"^{commit}"); err != nil {
			p.Err = fmt.Errorf("the fetched head of #%d does not hold %s", p.Number, head)
			return p
		}
		p.Into = head
	}
	return p
}

// gh is the Forge that runs the gh CLI in dir.
func gh(dir string) Forge {
	return func(args ...string) ([]byte, error) {
		cmd := exec.Command("gh", args...)
		cmd.Dir = dir
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			return out.Bytes(), fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
		}
		return out.Bytes(), nil
	}
}
