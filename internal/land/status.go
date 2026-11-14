package land

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Exit codes of `harness pr status`, one per blocker in the order address-review works them. 1 and 2 stay an operational failure and an invalid invocation.
const (
	ExitReady    = 0
	ExitConflict = 3
	ExitThread   = 4
	ExitFailing  = 5
	ExitPending  = 6
	ExitOther    = 7
	ExitTimeout  = 8
)

// Status is the verdict `harness pr status` prints for one pull request: whether it is ready to merge and what stands in the way. Its holds and review are the ones `harness land check --merge` reports for the same head.
type Status struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Head   string `json:"head"`
	Ready  bool   `json:"ready"`
	// Blocker names the first blocker: conflict, thread, failing, pending, or other. Empty when ready.
	Blocker  string    `json:"blocker,omitempty"`
	Review   *Review   `json:"review,omitempty"`
	Findings []Finding `json:"findings"`
	Threads  []Thread  `json:"threads"`
	Holds    []string  `json:"holds"`
	Notes    []string  `json:"notes"`
}

// Code is the exit code that names the status's first blocker.
func (s Status) Code() int {
	switch s.Blocker {
	case "":
		return ExitReady
	case "conflict":
		return ExitConflict
	case "thread":
		return ExitThread
	case "failing":
		return ExitFailing
	case "pending":
		return ExitPending
	}
	return ExitOther
}

// Statuses reports the pull requests in landing order, as `land check --merge` would check them, and never writes to the forge.
func Statuses(e Env, numbers []int, stack bool) ([]Status, error) {
	checks, err := Run(e, numbers, stack, true)
	if err != nil {
		return nil, err
	}
	permissions := map[string]string{}
	out := make([]Status, 0, len(checks))
	for _, c := range checks {
		s := Status{
			Number: c.Number, URL: c.URL, Title: c.Title, Head: c.Head,
			Review: c.Review, Findings: c.Findings, Threads: []Thread{}, Holds: c.Holds, Notes: c.Notes,
		}
		if c.Lane == "merged" {
			s.Holds = append([]string{"it is already merged"}, s.Holds...)
		}
		for _, t := range c.gate.threads {
			if _, ok := permissions[t.Author]; !ok {
				permissions[t.Author] = e.permission(t.Author)
			}
			t.Permission = permissions[t.Author]
			s.Threads = append(s.Threads, t)
		}
		switch {
		case len(s.Holds) == 0:
			s.Ready = true
		case c.gate.conflict:
			s.Blocker = "conflict"
		case len(c.gate.threads) > 0:
			s.Blocker = "thread"
		case c.gate.failing:
			s.Blocker = "failing"
		case c.gate.pending:
			s.Blocker = "pending"
		default:
			s.Blocker = "other"
		}
		out = append(out, s)
	}
	return out, nil
}

// permission reads a user's permission on the base repository, or unknown when the forge will not say, as for a caller without push access.
func (e Env) permission(login string) string {
	if login == "" {
		return "unknown"
	}
	raw, err := e.Forge("api", "repos/{owner}/{repo}/collaborators/"+login+"/permission", "--jq", ".permission")
	p := strings.TrimSpace(string(raw))
	if err != nil || p == "" {
		return "unknown"
	}
	// A --jq filter prints a string bare, but a raw response is JSON.
	var quoted string
	if json.Unmarshal(raw, &quoted) == nil {
		p = quoted
	}
	return p
}

// Code is the exit code of a run: the first pull request in landing order that is not ready names it.
func Code(statuses []Status) int {
	for _, s := range statuses {
		if !s.Ready {
			return s.Code()
		}
	}
	return ExitReady
}

// Clock is what a watch waits on, injected so a test never sleeps.
type Clock struct {
	Now   func() time.Time
	Sleep func(time.Duration)
}

// Watch repeats the status pass until the verdict is terminal, or until timeout passes, and returns the last verdict with its exit code: ExitTimeout when time ran out first. A failed pass is retried on the next one; three in a row end the watch with the last error.
func Watch(e Env, numbers []int, stack bool, interval, timeout time.Duration, clock Clock) ([]Status, int, error) {
	deadline := clock.Now().Add(timeout)
	var last []Status
	failures := 0
	for {
		statuses, err := Statuses(e, numbers, stack)
		switch {
		case err != nil:
			if failures++; failures == 3 {
				return last, 1, fmt.Errorf("three status passes in a row failed: %w", err)
			}
		default:
			failures, last = 0, statuses
			if terminal(statuses) {
				return statuses, Code(statuses), nil
			}
		}
		if !clock.Now().Add(interval).Before(deadline) {
			if last == nil {
				return nil, 1, fmt.Errorf("timed out before a status pass succeeded: %w", err)
			}
			return last, ExitTimeout, nil
		}
		clock.Sleep(interval)
	}
}

// terminal reports whether the run's first blocker, the one its exit code names, is anything but pending required checks.
func terminal(statuses []Status) bool {
	return Code(statuses) != ExitPending
}
