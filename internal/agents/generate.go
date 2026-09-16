package agents

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func generate(outputs map[string]string, output string, report []string, stdout io.Writer) int {
	changed := false
	for relative, path := range existing(output) {
		if _, ok := outputs[relative]; !ok {
			_ = os.Remove(path)
			changed = true
		}
	}
	names := make([]string, 0, len(outputs))
	for relative := range outputs {
		names = append(names, relative)
	}
	sort.Strings(names)
	for _, relative := range names {
		text := outputs[relative]
		path := filepath.Join(output, filepath.FromSlash(relative))
		if raw, err := os.ReadFile(path); err == nil && string(raw) == text {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintln(stdout, err)
			return 4
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			fmt.Fprintln(stdout, err)
			return 4
		}
		changed = true
	}
	lines := append(append(append([]string{}, report...), gaps...), unverified...)
	fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	if changed {
		return 0
	}
	return 3
}
