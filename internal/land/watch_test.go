package land

import (
	"errors"
	"testing"
	"time"
)

// fakeClock advances only when the watch sleeps, so no test waits.
type fakeClock struct {
	now    time.Time
	sleeps int
}

func (c *fakeClock) clock() Clock {
	return Clock{Now: func() time.Time { return c.now }, Sleep: func(d time.Duration) { c.sleeps++; c.now = c.now.Add(d) }}
}

// scripted wraps the fixture's forge so each status pass sees the next scripted change before it reads.
func (f *fixture) watch(timeout time.Duration, before func(pass int)) ([]Status, int, error, *fakeClock) {
	f.t.Helper()
	pass := 0
	forge := func(args ...string) ([]byte, error) {
		if args[0] == "repo" { // the first read of every pass
			pass++
			before(pass)
		}
		return f.forge(args...)
	}
	c := &fakeClock{now: time.Unix(0, 0)}
	statuses, code, err := Watch(Env{Dir: f.dir, Forge: forge, State: f.state}, []int{1}, false, time.Minute, timeout, c.clock())
	return statuses, code, err, c
}

// TestWatch covers waiting through pending required checks, and stopping at once on any other verdict.
func TestWatch(t *testing.T) {
	f := setup(t)
	f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	pending := func() {
		f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"pending"}]`, errors.New("exit status 8")
	}
	passing := func() { f.checks[1] = `[{"name":"ci","bucket":"pass"}]`; delete(f.checksErr, 1) }
	failing := func() { f.checks[1], f.checksErr[1] = `[{"name":"ci","bucket":"fail"}]`, errors.New("exit status 1") }

	t.Run("pending then passing", func(t *testing.T) {
		statuses, code, err, c := f.watch(time.Hour, func(pass int) {
			if pass < 3 {
				pending()
			} else {
				passing()
			}
		})
		if err != nil || code != ExitReady || !statuses[0].Ready || c.sleeps != 2 {
			t.Fatalf("code %d, sleeps %d, err %v", code, c.sleeps, err)
		}
	})
	t.Run("turns red", func(t *testing.T) {
		_, code, _, c := f.watch(time.Hour, func(pass int) {
			if pass < 2 {
				pending()
			} else {
				failing()
			}
		})
		if code != ExitFailing || c.sleeps != 1 {
			t.Fatalf("code %d after %d sleeps", code, c.sleeps)
		}
	})
	t.Run("thread on the first pass", func(t *testing.T) {
		defer func() { f.threads[1] = 0 }()
		_, code, _, c := f.watch(time.Hour, func(int) { pending(); f.threads[1] = 1 })
		if code != ExitThread || c.sleeps != 0 {
			t.Fatalf("code %d after %d sleeps", code, c.sleeps)
		}
	})
	t.Run("conflict on the first pass", func(t *testing.T) {
		defer func() { f.pulls[1].Mergeable = "MERGEABLE" }()
		_, code, _, c := f.watch(time.Hour, func(int) { pending(); f.pulls[1].Mergeable = "CONFLICTING" })
		if code != ExitConflict || c.sleeps != 0 {
			t.Fatalf("code %d after %d sleeps", code, c.sleeps)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		statuses, code, err, _ := f.watch(3*time.Minute, func(int) { pending() })
		if err != nil || code != ExitTimeout || len(statuses) != 1 || statuses[0].Blocker != "pending" {
			t.Fatalf("code %d, err %v, last %+v", code, err, statuses)
		}
	})
}

// TestWatchRetries covers a failed forge read retried on the next pass, and three in a row ending the watch.
func TestWatchRetries(t *testing.T) {
	f := setup(t)
	f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	c := &fakeClock{now: time.Unix(0, 0)}
	pass := 0
	flaky := func(fail int) Forge {
		pass = 0
		return func(args ...string) ([]byte, error) {
			if args[0] == "repo" {
				pass++
			}
			if pass <= fail {
				return nil, errors.New("HTTP 502")
			}
			return f.forge(args...)
		}
	}
	if _, code, err := Watch(Env{Dir: f.dir, Forge: flaky(2), State: f.state}, []int{1}, false, time.Minute, time.Hour, c.clock()); code != ExitReady || err != nil {
		t.Errorf("two failures then a pass: code %d, err %v", code, err)
	}
	if _, code, err := Watch(Env{Dir: f.dir, Forge: flaky(3), State: f.state}, []int{1}, false, time.Minute, time.Hour, c.clock()); code != 1 || err == nil || pass != 3 {
		t.Errorf("three failures: code %d after %d passes, err %v", code, pass, err)
	}
}
