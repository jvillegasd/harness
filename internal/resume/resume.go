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
       harness resume init

Print the task context a client restores after compacting a session: the
branch, its head commit, and the state and next action of every task
document under docs/agents/tasks/: each local one, ignored or not, and each
committed one the branch changed. The client hooks the harness installs run
it from the session's working directory.

It prints nothing outside a Git checkout and always exits 0, so a hook never
blocks compaction.

init keeps task documents local: it adds docs/agents/tasks/ to this clone's
.git/info/exclude, which every worktree of the clone shares, so a task
document is never committed. Running it again changes nothing.
`

const tasksDir = "docs/agents/tasks/"

const maxSectionLines = 40

func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		if args[0] == "-h" || args[0] == "--help" {
			fmt.Fprint(stdout, usage)
			return 0
		}
		if args[0] == "init" && len(args) == 1 {
			dir, err := os.Getwd()
			if err == nil {
				err = Exclude(dir)
			}
			if err != nil {
				fmt.Fprintf(stderr, "harness resume init: %v\n", err)
				return 1
			}
			fmt.Fprintf(stdout, "%s is excluded from Git in this clone\n", tasksDir)
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
	fmt.Fprintln(&b, "\nResume per `checkpoint`: read the task document and the branch's commits, trust evidence bound to unchanged commits, and rerun only what a later change made stale.")
	return b.String()
}

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
	out, _ := git(dir, "ls-files", "--others", "--", tasksDir)
	add(out)
	return docs
}

// Exclude adds the task documents folder to the clone's info/exclude, which
// the clone's worktrees share, so task documents stay local.
func Exclude(dir string) error {
	path, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}
	path = filepath.Join(path, "info", "exclude")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	line := "/" + tasksDir
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == line {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	data = append(data, []byte(line+"\n")...)
	return os.WriteFile(path, data, 0o644)
}

func mergeBase(dir string) string {
	for _, ref := range []string{"origin/HEAD", "main", "master"} {
		if base, err := git(dir, "merge-base", "HEAD", ref); err == nil {
			return base
		}
	}
	return ""
}

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
