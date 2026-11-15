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

func errNotLocal(c Config) error {
	if c.Tracker == "local-markdown" {
		return nil
	}
	return fmt.Errorf("%s selects the %q tracker, not local-markdown; its transitions are the tracker's own", ConfigPath, c.Tracker)
}
