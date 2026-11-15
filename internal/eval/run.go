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

type Session struct {
	Label   string   `json:"label"`
	Model   string   `json:"model"`
	Dir     string   `json:"dir"`
	Copy    string   `json:"copy_of,omitempty"`
	Command []string `json:"command"`
	Status  string   `json:"status,omitempty"`
	Exit    int      `json:"exit,omitempty"`
	Error   string   `json:"error,omitempty"`
	Output  string   `json:"output,omitempty"`
	Log     string   `json:"log,omitempty"`
	Record  string   `json:"record,omitempty"`
	stdin   bool
	session string
}

type RunOptions struct {
	PromptFile string
	Models     []string
	DryRun     bool
}

const sessionsFile = "sessions.json"

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
