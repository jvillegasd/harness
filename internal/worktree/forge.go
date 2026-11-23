package worktree

import (
	"encoding/json"
	"fmt"
	"github.com/jvillegasd/harness/internal/run"
	"strconv"
)

type Pull struct {
	Number int
	State  string
	Into   string
	Err    error
}

func (p Pull) Held() bool { return p.State == "OPEN" }

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

func ReadPull(dir string, s Status, forge run.Forge) Pull {
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
		if _, err := run.Git(dir, "fetch", "--quiet", "origin", "refs/pull/"+strconv.Itoa(p.Number)+"/head"); err != nil {
			p.Err = err
			return p
		}
		if _, err := run.Git(dir, "cat-file", "-e", head+"^{commit}"); err != nil {
			p.Err = fmt.Errorf("the fetched head of #%d does not hold %s", p.Number, head)
			return p
		}
		p.Into = head
	}
	return p
}
