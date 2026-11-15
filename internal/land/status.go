package land

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	ExitReady    = 0
	ExitConflict = 3
	ExitThread   = 4
	ExitFailing  = 5
	ExitPending  = 6
	ExitOther    = 7
	ExitTimeout  = 8
)

type Status struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Head   string `json:"head"`
	Ready  bool   `json:"ready"`

	Blocker string  `json:"blocker,omitempty"`
	Review  *Review `json:"review,omitempty"`

	Run     *RunVerdict `json:"run,omitempty"`
	Threads []Thread    `json:"threads"`
	Holds   []string    `json:"holds"`
	Notes   []string    `json:"notes"`
}

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
			Review: c.Review, Run: c.RunVerdict, Threads: []Thread{}, Holds: c.Holds, Notes: c.Notes,
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
		case c.gate.pending || c.gate.computing:
			s.Blocker = "pending"
		default:
			s.Blocker = "other"
		}
		out = append(out, s)
	}
	return out, nil
}

func (e Env) permission(login string) string {
	if login == "" {
		return "unknown"
	}
	raw, err := e.Forge("api", "repos/{owner}/{repo}/collaborators/"+login+"/permission", "--jq", ".permission")
	p := strings.TrimSpace(string(raw))
	if err != nil || p == "" {
		return "unknown"
	}

	var quoted string
	if json.Unmarshal(raw, &quoted) == nil {
		p = quoted
	}
	return p
}

func Code(statuses []Status) int {
	for _, s := range statuses {
		if !s.Ready {
			return s.Code()
		}
	}
	return ExitReady
}

type Clock struct {
	Now   func() time.Time
	Sleep func(time.Duration)
}

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

func terminal(statuses []Status) bool {
	return Code(statuses) != ExitPending
}
