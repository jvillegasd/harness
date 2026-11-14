package eval

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	mrand "math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Session is one headless client session in one candidate directory.
type Session struct {
	Label   string   `json:"label"`
	Model   string   `json:"model"`
	Dir     string   `json:"dir"`
	Copy    string   `json:"copy_of,omitempty"` // the planted directory a later model's copy was made from
	Command []string `json:"command"`
	Status  string   `json:"status,omitempty"` // ok or failed
	Exit    int      `json:"exit,omitempty"`
	Error   string   `json:"error,omitempty"`
	Output  string   `json:"output,omitempty"` // stdout, saved by label outside every candidate directory
	Log     string   `json:"log,omitempty"`    // stderr, beside it
	Record  string   `json:"record,omitempty"` // the session record the client wrote, when found
	stdin   bool
	session string
}

// RunOptions is what `harness eval run` takes.
type RunOptions struct {
	PromptFile string
	Models     []string
	DryRun     bool
}

// sessionsFile records a run's sessions; its presence means the run already ran.
const sessionsFile = "sessions.json"

// Plan lays out one session per planted directory and model. The first model runs in the planted directories; each later model gets a fresh copy of every one, under a new ordinary name and a new label in shuffled order, so each model appears in both arms and no two sessions share a directory.
func Plan(m Manifest, k Key, prompt string, models []string) ([]Session, Key, error) {
	planted := append([]Planted(nil), m.Dirs...)
	sort.Slice(planted, func(i, j int) bool { return labelNumber(planted[i].Label) < labelNumber(planted[j].Label) })
	next := 0
	for _, p := range m.Dirs {
		next = max(next, labelNumber(p.Label))
	}
	key := Key{Run: k.Run, Labels: map[string]string{}}
	for l, a := range k.Labels {
		key.Labels[l] = a
	}
	var sessions []Session
	taken := map[string]bool{}
	for i, model := range models {
		order := append([]Planted(nil), planted...)
		if i > 0 {
			mrand.Shuffle(len(order), func(a, b int) { order[a], order[b] = order[b], order[a] })
		}
		for _, p := range order {
			s := Session{Label: p.Label, Model: model, Dir: p.Dir}
			if i > 0 {
				dir, err := freshDir(m.Root)
				for err == nil && taken[dir] {
					dir, err = freshDir(m.Root)
				}
				if err != nil {
					return nil, key, err
				}
				taken[dir] = true
				next++
				s.Label, s.Dir, s.Copy = fmt.Sprintf("r%d", next), dir, p.Dir
				key.Labels[s.Label] = k.Labels[p.Label]
			}
			s.Command, s.stdin, s.session = command(m.Client, model, prompt)
			sessions = append(sessions, s)
		}
	}
	return sessions, key, nil
}

func labelNumber(label string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(label, "r"))
	return n
}

// command is a client's headless form in the mode that lets it edit only its working directory: Claude Code's acceptEdits, Codex's workspace-write sandbox, and OpenCode's --auto under the planted opencode.json that denies edits outside the directory. Claude Code and Codex read the prompt on stdin; OpenCode takes it as its message.
func command(client, model, prompt string) ([]string, bool, string) {
	switch client {
	case "claude":
		id := uuid()
		return []string{"claude", "-p", "--permission-mode", "acceptEdits", "--model", model, "--session-id", id}, true, id
	case "codex":
		return []string{"codex", "exec", "--sandbox", "workspace-write", "--skip-git-repo-check", "--model", model, "-"}, true, ""
	}
	return []string{"opencode", "run", "--auto", "--model", model, "--", prompt}, false, ""
}

func uuid() string {
	h := []byte(randomHex(16))
	h[12] = '4'
	h[16] = "89ab"[mrand.IntN(4)]
	return string(h[0:8]) + "-" + string(h[8:12]) + "-" + string(h[12:16]) + "-" + string(h[16:20]) + "-" + string(h[20:32])
}

// Shell renders a session's command as one line a human can approve, with the prompt file on stdin where the client reads it there.
func (s Session) Shell(promptFile string) string {
	parts := make([]string, len(s.Command))
	for i, arg := range s.Command {
		if !s.stdin && i == len(s.Command)-1 {
			parts[i] = `"$(cat ` + quote(promptFile) + `)"`
			continue
		}
		parts[i] = quote(arg)
	}
	line := "cd " + quote(s.Dir) + " && " + strings.Join(parts, " ")
	if s.stdin {
		line += " < " + quote(promptFile)
	}
	return line
}

var plainArg = regexp.MustCompile(`^[A-Za-z0-9_./:=@+-]+$`)

func quote(s string) string {
	if plainArg.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Run starts every planned session at once, each in its own directory with the prompt's exact text, waits for all of them, and records each one's exit, output, and session record. A session that fails is recorded and named and does not stop the others. With DryRun it prints each command and starts and writes nothing.
func Run(home, run string, o RunOptions, out io.Writer) ([]Session, error) {
	m, k, err := ReadRun(home, run)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(o.PromptFile)
	if err != nil {
		return nil, fmt.Errorf("prompt file: %w", err)
	}
	if len(o.Models) == 0 {
		return nil, errors.New("name at least one --model")
	}
	meta := MetaDir(home, run)
	if _, err := os.Stat(filepath.Join(meta, sessionsFile)); err == nil {
		return nil, fmt.Errorf("run %s already ran; plant a new run", run)
	}
	prompt := string(raw)
	sessions, key, err := Plan(m, k, prompt, o.Models)
	if err != nil {
		return nil, err
	}
	if o.DryRun {
		for _, s := range sessions {
			note := ""
			if s.Copy != "" {
				note = " (a copy of " + s.Copy + ")"
			}
			fmt.Fprintf(out, "%s %s%s\n  %s\n", s.Label, s.Model, note, s.Shell(o.PromptFile))
		}
		return sessions, nil
	}
	// Copies join the manifest and the key before any is made, so clean covers a run that stops partway.
	for _, s := range sessions {
		if s.Copy != "" {
			m.Dirs = append(m.Dirs, Planted{s.Label, s.Dir})
		}
	}
	if err := writeJSON(filepath.Join(meta, "manifest.json"), m); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(meta, "key.json"), key); err != nil {
		return nil, err
	}
	for _, s := range sessions {
		if s.Copy != "" {
			if err := os.Mkdir(s.Dir, 0o755); err != nil {
				return nil, err
			}
			if err := copyTree(s.Copy, s.Dir); err != nil {
				return nil, err
			}
		}
	}
	outputs := filepath.Join(meta, "outputs")
	if err := os.MkdirAll(outputs, 0o700); err != nil {
		return nil, err
	}
	start := time.Now()
	var wg sync.WaitGroup
	for i := range sessions {
		wg.Add(1)
		go func(s *Session) {
			defer wg.Done()
			start1(s, prompt, outputs)
		}(&sessions[i])
	}
	wg.Wait()
	for i := range sessions {
		sessions[i].Record = record(home, m.Client, &sessions[i], start)
	}
	if err := writeJSON(filepath.Join(meta, sessionsFile), sessions); err != nil {
		return sessions, err
	}
	return sessions, nil
}

// start1 runs one session to its end.
func start1(s *Session, prompt, outputs string) {
	s.Output = filepath.Join(outputs, s.Label+".out")
	s.Log = filepath.Join(outputs, s.Label+".err")
	stdout, err := os.Create(s.Output)
	if err != nil {
		s.Status, s.Error = "failed", err.Error()
		return
	}
	defer stdout.Close()
	stderr, err := os.Create(s.Log)
	if err != nil {
		s.Status, s.Error = "failed", err.Error()
		return
	}
	defer stderr.Close()
	cmd := exec.Command(s.Command[0], s.Command[1:]...)
	cmd.Dir = s.Dir
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if s.stdin {
		cmd.Stdin = strings.NewReader(prompt)
	}
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		s.Status = "ok"
	case errors.As(err, &exit):
		s.Status, s.Exit = "failed", exit.ExitCode()
	default:
		s.Status, s.Error = "failed", err.Error()
	}
}

// record finds the session record the client wrote for this session: Claude Code's transcript under its projects directory, named by the session id it was given; the newest Codex rollout since the run started whose session metadata names this directory; and OpenCode's database, which holds every session.
func record(home, client string, s *Session, since time.Time) string {
	switch client {
	case "claude":
		config := os.Getenv("CLAUDE_CONFIG_DIR")
		if config == "" {
			config = filepath.Join(home, ".claude")
		}
		dir := s.Dir
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		path := filepath.Join(config, "projects", slug(dir), s.session+".jsonl")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	case "codex":
		root := os.Getenv("CODEX_HOME")
		if root == "" {
			root = filepath.Join(home, ".codex")
		}
		return codexRollout(filepath.Join(root, "sessions"), s.Dir, since)
	case "opencode":
		data := os.Getenv("XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(home, ".local", "share")
		}
		path := filepath.Join(data, "opencode", "opencode.db")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// slug is Claude Code's projects directory name for a working directory.
func slug(dir string) string { return nonAlnum.ReplaceAllString(dir, "-") }

func codexRollout(root, dir string, since time.Time) string {
	want, _ := json.Marshal(dir)
	best, bestTime := "", time.Time{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "rollout-") || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().Before(since) {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		r := bufio.NewReaderSize(f, 1<<16)
		line, _ := r.ReadString('\n')
		if strings.Contains(line, `"cwd":`+string(want)) && info.ModTime().After(bestTime) {
			best, bestTime = path, info.ModTime()
		}
		return nil
	})
	return best
}
