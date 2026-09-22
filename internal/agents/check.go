package agents

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RestrictionsMatch checks the complete rendered permission configuration.
func RestrictionsMatch(relative, text, level string) bool {
	lines := splitLines(text)
	if strings.HasPrefix(relative, "claude/") {
		head := lines
		if countExact(lines, "---") >= 2 {
			index := -1
			for i := 1; i < len(lines); i++ {
				if lines[i] == "---" {
					index = i
					break
				}
			}
			if index >= 0 {
				head = lines[:index]
			}
		}
		var kept []string
		for _, line := range head {
			if line == "---" || line == "" || strings.HasPrefix(line, "name:") || strings.HasPrefix(line, "description:") || strings.HasPrefix(line, "model:") || strings.HasPrefix(line, "effort:") {
				continue
			}
			kept = append(kept, line)
		}
		want := claudeControls[level]
		if len(kept) != len(want) {
			return false
		}
		for i := range kept {
			if kept[i] != want[i] {
				return false
			}
		}
		return true
	}
	head := ""
	if strings.HasPrefix(text, "---\n") {
		parts := strings.Split(text, "---\n")
		if len(parts) > 1 {
			head = parts[1]
		}
	}
	kept := ""
	for _, line := range splitLines(head) {
		if strings.HasPrefix(line, "description:") || strings.HasPrefix(line, "model:") || strings.HasPrefix(line, "reasoningEffort:") {
			continue
		}
		kept += line + "\n"
	}
	return kept == "mode: all\n"+opencodeControls[level]
}

func existing(output string) map[string]string {
	found := map[string]string{}
	info, err := os.Stat(output)
	if err != nil || !info.IsDir() {
		return found
	}
	_ = filepath.WalkDir(output, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(output, path)
		if err != nil {
			return nil
		}
		found[filepath.ToSlash(rel)] = path
		return nil
	})
	return found
}

func checkSymlinks(outputs map[string]string, output string, stderr io.Writer) int {
	names := make([]string, 0, len(outputs))
	for relative := range outputs {
		names = append(names, relative)
	}
	sort.Strings(names)
	for _, relative := range names {
		path := filepath.Join(output, filepath.FromSlash(relative))
		entries := append([]string{path}, parents(path)...)
		for _, entry := range entries {
			if info, err := os.Lstat(entry); err == nil && info.Mode()&os.ModeSymlink != 0 {
				fmt.Fprintf(stderr, "agents: %s: symlink in role agent output path\n", entry)
				return 1
			}
		}
	}
	return 0
}

// parents lists path's ancestors from its parent up to the filesystem root.
func parents(path string) []string {
	var out []string
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		out = append(out, dir)
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return out
}

// checkOutputs reports each output that is missing, drifted, unmanaged, or widened, returning 1 when it found any.
func checkOutputs(outputs, accesses map[string]string, output string, stdout io.Writer) int {
	present := existing(output)
	var findings []string
	for relative := range present {
		if _, ok := outputs[relative]; !ok {
			findings = append(findings, "unmanaged: "+relative)
		}
	}
	sort.Strings(findings)
	names := make([]string, 0, len(outputs))
	for relative := range outputs {
		names = append(names, relative)
	}
	sort.Strings(names)
	for _, relative := range names {
		expected := outputs[relative]
		path, ok := present[relative]
		if !ok {
			findings = append(findings, "missing: "+relative)
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			findings = append(findings, "missing: "+relative)
			continue
		}
		if !RestrictionsMatch(relative, string(raw), accesses[relative]) {
			findings = append(findings, "widened: "+relative)
		} else if string(raw) != expected {
			findings = append(findings, "changed: "+relative)
		}
	}
	if len(findings) > 0 {
		fmt.Fprintln(stdout, strings.Join(findings, "\n"))
		return 1
	}
	return 0
}
