// Package resume prints the task context a client's compaction hook restores: the branch, its head commit, and the state and next action of each task document the branch changed. It is the target of the SessionStart (compact) hooks in Claude Code and Codex and of the OpenCode compaction plugin.
package resume

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const usage = `Usage: harness resume

Print the task context a client restores after compacting a session: the
branch, its head commit, and the state and next action of every task
document under docs/agents/tasks/ that the branch changed. The client hooks
the harness installs run it from the session's working directory.

It prints nothing outside a Git checkout and always exits 0, so a hook never
blocks compaction.
`

// tasksDir is where continuity keeps task documents.
const tasksDir = "docs/agents/tasks/"

// maxSectionLines caps each document's excerpt, so a long document cannot flood the restored context.
const maxSectionLines = 40

// Command runs `harness resume` in the current directory.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		if args[0] == "-h" || args[0] == "--help" {
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprintf(stderr, "harness resume: unexpected argument %q\n%s", args[0], usage)
		return 2
	}
	dir, err := os.Getwd()
	if err != nil {
		return 0
	}
	fmt.Fprint(stdout, Context(dir))
	return 0
}

// Context renders the restored context for the checkout holding dir, or "" when dir is not in one.
func Context(dir string) string {
	root, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	dir = root
	branch, err := git(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	head, _ := git(dir, "log", "-1", "--format=%h %s")
	var b strings.Builder
	fmt.Fprintln(&b, "Task context restored after compaction:")
	fmt.Fprintf(&b, "- branch: %s\n- HEAD: %s\n", branch, head)
	docs := changedTaskDocs(dir)
	if len(docs) == 0 {
		fmt.Fprintln(&b, "- task document: none changed on this branch")
	}
	for _, doc := range docs {
		fmt.Fprintf(&b, "- task document: %s\n", doc)
		if section := nextAction(dir, doc); section != "" {
			fmt.Fprintf(&b, "\n%s\n", section)
		}
	}
	fmt.Fprintln(&b, "\nResume per `continuity`: read the task document and the branch's commits, trust evidence bound to unchanged commits, and rerun only what a later change made stale.")
	return b.String()
}

// changedTaskDocs runs at the top of the checkout, so its pathspecs and the paths it prints are top-relative. It lists the task documents that differ from the default branch's merge base, committed or not, plus untracked ones. On the default branch itself nothing differs, so nothing is listed.
func changedTaskDocs(dir string) []string {
	var docs []string
	seen := map[string]bool{}
	add := func(out string) {
		for _, path := range strings.Split(out, "\n") {
			if strings.HasSuffix(path, ".md") && !seen[path] {
				seen[path] = true
				docs = append(docs, path)
			}
		}
	}
	if base := mergeBase(dir); base != "" {
		out, _ := git(dir, "diff", "--name-only", "--diff-filter=d", base, "--", tasksDir)
		add(out)
	}
	out, _ := git(dir, "ls-files", "--others", "--exclude-standard", "--", tasksDir)
	add(out)
	return docs
}

// mergeBase finds where the branch left the default branch, trying the remote's default before a local main or master.
func mergeBase(dir string) string {
	for _, ref := range []string{"origin/HEAD", "main", "master"} {
		if base, err := git(dir, "merge-base", "HEAD", ref); err == nil {
			return base
		}
	}
	return ""
}

// nextAction returns the document's "State and next action" section, heading included, capped at maxSectionLines lines.
func nextAction(root, doc string) string {
	data, err := os.ReadFile(filepath.Join(root, doc))
	if err != nil {
		return ""
	}
	var lines []string
	in := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") {
			if in {
				break
			}
			in = strings.Contains(strings.ToLower(line), "next action")
		}
		if in {
			if len(lines) == maxSectionLines {
				lines = append(lines, "[...]")
				break
			}
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}
