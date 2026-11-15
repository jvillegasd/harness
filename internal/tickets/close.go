package tickets

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var statusLineRe = regexp.MustCompile(`(?i)^status:\s*(.*?)\s*$`)

// status finds a file's Status: line before its first section: the line's index and its value, or -1 when it has none.
func status(lines []string) (int, string) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		if m := statusLineRe.FindStringSubmatch(trimmed); m != nil {
			return i, m[1]
		}
	}
	return -1, ""
}

// setStatus gives the file at path the Status: value, replacing its Status: line or adding one under the title, and reports whether the file changed.
func setStatus(path, value string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(raw), "\n")
	i, current := status(lines)
	switch {
	case i >= 0 && current == value:
		return false, nil
	case i >= 0:
		lines[i] = "Status: " + value
	default:
		title := 0
		for title < len(lines)-1 && strings.TrimSpace(lines[title]) == "" {
			title++
		}
		lines = append(lines[:title+1], append([]string{"", "Status: " + value}, lines[title+1:]...)...)
	}
	return true, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// Close applies a local-markdown tracker's transitions after the named tickets merged: each becomes done, each ticket the mapped blocked label held whose blockers are now all done moves to the mapped ready_for_agent label, and the feature's spec.md becomes done once every ticket is. dir is the feature's issues directory under root. It returns each path it changed, relative to root; a second run changes nothing.
func Close(c Config, root, dir string, ts []Ticket, merged []int) ([]string, error) {
	f, err := Compute(ts, merged)
	if err != nil {
		return nil, err
	}
	blocked, ready := c.label("blocked", "blocked"), c.label("ready_for_agent", "ready-for-agent")
	done := map[int]bool{}
	for _, n := range f.Done {
		done[n] = true
	}
	isReady := map[int]bool{}
	for _, n := range f.Ready {
		isReady[n] = true
	}
	var changed []string
	set := func(rel, value string) error {
		ok, err := setStatus(filepath.Join(root, rel), value)
		if ok {
			changed = append(changed, rel)
		}
		return err
	}
	for _, t := range ts {
		switch {
		case done[t.Number] && !t.Done:
			err = set(t.Path, "done")
		case isReady[t.Number]:
			raw, rerr := os.ReadFile(filepath.Join(root, t.Path))
			if rerr != nil {
				return changed, rerr
			}
			// Only a ticket held as blocked advances; one claimed or handed off keeps its state.
			if _, current := status(strings.Split(string(raw), "\n")); strings.EqualFold(current, blocked) {
				err = set(t.Path, ready)
			}
		}
		if err != nil {
			return changed, err
		}
	}
	if len(f.Done) == len(ts) {
		spec := filepath.Join(filepath.Dir(dir), "spec.md")
		if _, err := os.Stat(filepath.Join(root, spec)); err == nil {
			if err := set(spec, "done"); err != nil {
				return changed, err
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return changed, err
		}
	}
	return changed, nil
}

// errNotLocal refuses a tracker whose state does not live in committed files.
func errNotLocal(c Config) error {
	if c.Tracker == "local-markdown" {
		return nil
	}
	return fmt.Errorf("%s selects the %q tracker, not local-markdown; its transitions are the tracker's own", ConfigPath, c.Tracker)
}
