package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	harness "github.com/jvillegasd/harness"
)

type Forge func(args ...string) ([]byte, error)

var (
	ConfigPath     = filepath.Join("docs", "agents", "harness.json")
	WorkflowPath   = filepath.Join(".github", "workflows", "agent-lifecycle.yml")
	ReconcilerPath = filepath.Join(".github", "scripts", "reconcile-agent-lifecycle.go")
	supersededPath = filepath.Join(".github", "scripts", "reconcile-agent-lifecycle.py")
)

const (
	workflowAsset   = "skills/ops/setup-harness/assets/agent-lifecycle.yml"
	reconcilerAsset = "skills/ops/setup-harness/scripts/reconcile-agent-lifecycle.go"
)

var Assets fs.FS = harness.Assets

var labelKeys = [][2]string{{"blocked", "blocked"}, {"ready_for_agent", "ready-for-agent"}, {"in_progress", "in-progress"}, {"ready_for_human", "ready-for-human"}}

var specTrees = []string{filepath.Join("docs", "specs"), "specs"}

type Evidence struct {
	GitHubRemote   bool     `json:"github_remote"`
	Remote         string   `json:"remote,omitempty"`
	RemoteURL      string   `json:"remote_url,omitempty"`
	DefaultBranch  string   `json:"default_branch,omitempty"`
	Protected      *bool    `json:"protected"`
	ProtectedError string   `json:"protected_error,omitempty"`
	ConfiguredMode string   `json:"configured_mode,omitempty"`
	SpecHome       string   `json:"spec_home,omitempty"`
	LocalSpecTrees []string `json:"local_spec_trees"`
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func Probe(root string, forge Forge) Evidence {
	e := Evidence{LocalSpecTrees: []string{}}
	remotes, _ := git(root, "remote")
	names := strings.Fields(remotes)
	sort.SliceStable(names, func(i, j int) bool { return names[i] == "origin" && names[j] != "origin" })
	for _, name := range names {
		url, err := git(root, "remote", "get-url", name)
		if err == nil && strings.Contains(url, "github.com") {
			e.GitHubRemote, e.Remote, e.RemoteURL = true, name, url
			break
		}
	}
	if e.GitHubRemote {
		if head, err := git(root, "symbolic-ref", "--short", "refs/remotes/"+e.Remote+"/HEAD"); err == nil {
			e.DefaultBranch = strings.TrimPrefix(head, e.Remote+"/")
		} else if out, err := forge("repo", "view", "--json", "defaultBranchRef"); err == nil {
			var repo struct{ DefaultBranchRef struct{ Name string } }
			if json.Unmarshal(out, &repo) == nil {
				e.DefaultBranch = repo.DefaultBranchRef.Name
			}
		}
		if e.DefaultBranch == "" {
			e.ProtectedError = "the default branch is unknown"
		} else if out, err := forge("api", "repos/{owner}/{repo}/branches/"+e.DefaultBranch); err != nil {
			e.ProtectedError = err.Error()
		} else {
			var branch struct{ Protected bool }
			if err := json.Unmarshal(out, &branch); err != nil {
				e.ProtectedError = err.Error()
			} else {
				e.Protected = &branch.Protected
			}
		}
	}
	if c, err := readConfig(root); err == nil {
		e.ConfiguredMode, e.SpecHome = c.str("tracker"), c.str("spec_home")
	}
	trees := append([]string{}, specTrees...)
	if e.SpecHome != "" && e.SpecHome != "github-issues" && !filepath.IsAbs(e.SpecHome) {
		trees = append(trees, filepath.Clean(e.SpecHome))
	}
	seen := map[string]bool{}
	for _, t := range trees {
		if info, err := os.Stat(filepath.Join(root, t)); err == nil && info.IsDir() && !seen[t] {
			seen[t] = true
			e.LocalSpecTrees = append(e.LocalSpecTrees, t)
		}
	}
	return e
}

type config map[string]json.RawMessage

func (c config) str(key string) string {
	var s string
	_ = json.Unmarshal(c[key], &s)
	return s
}

func readConfig(root string) (config, error) {
	raw, err := os.ReadFile(filepath.Join(root, ConfigPath))
	if err != nil {
		return nil, err
	}
	c := config{}
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", ConfigPath, err)
	}
	return c, nil
}

func object(c map[string]json.RawMessage, first []string, indent string) ([]byte, error) {
	var keys []string
	placed := map[string]bool{}
	for _, k := range first {
		if _, ok := c[k]; ok {
			keys, placed[k] = append(keys, k), true
		}
	}
	var rest []string
	for k := range c {
		if !placed[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	keys = append(keys, rest...)
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, k := range keys {
		name, _ := json.Marshal(k)
		var value bytes.Buffer
		if err := json.Indent(&value, c[k], indent+"  ", "  "); err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		b.WriteString(indent + "  " + string(name) + ": " + value.String())
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(indent + "}")
	return b.Bytes(), nil
}

type Options struct {
	Mode     string
	SpecHome string
}

func Tracker(root string, o Options, forge Forge) ([]string, error) {
	switch o.Mode {
	case "github":
		if o.SpecHome != "" {
			return nil, errors.New("--spec-home applies only to local-markdown")
		}
		if _, err := forge("auth", "status"); err != nil {
			return nil, fmt.Errorf("forge credentials: %w; run gh auth login", err)
		}
	case "local-markdown":
		if o.SpecHome == "" {
			o.SpecHome = filepath.Join("docs", "specs")
		}
	default:
		return nil, fmt.Errorf("unknown mode %q; use github or local-markdown", o.Mode)
	}
	c, err := readConfig(root)
	if errors.Is(err, fs.ErrNotExist) {
		c, err = config{}, nil
	}
	if err != nil {
		return nil, err
	}
	specHome := "github-issues"
	if o.Mode == "local-markdown" {
		specHome = filepath.ToSlash(o.SpecHome)
	}
	c["tracker"], _ = json.Marshal(o.Mode)
	c["spec_home"], _ = json.Marshal(specHome)
	labels := map[string]json.RawMessage{}
	if raw, ok := c["labels"]; ok {
		if err := json.Unmarshal(raw, &labels); err != nil {
			return nil, fmt.Errorf("%s: labels: %w", ConfigPath, err)
		}
	}
	var order, mapped []string
	for _, kv := range labelKeys {
		order = append(order, kv[0])
		if _, ok := labels[kv[0]]; !ok {
			labels[kv[0]], _ = json.Marshal(kv[1])
		}
		var value string
		if err := json.Unmarshal(labels[kv[0]], &value); err != nil || value == "" {
			return nil, fmt.Errorf("%s: label %s must be a non-empty string", ConfigPath, kv[0])
		}
		mapped = append(mapped, value)
	}
	inner, err := object(labels, order, "  ")
	if err != nil {
		return nil, err
	}
	c["labels"] = inner
	text, err := object(c, []string{"tracker", "spec_home", "labels"}, "")
	if err != nil {
		return nil, err
	}
	var done []string
	if err := write(root, ConfigPath, append(text, '\n'), &done); err != nil {
		return done, err
	}
	if o.Mode != "github" {
		return done, nil
	}
	if err := ensureLabels(forge, mapped, &done); err != nil {
		return done, err
	}
	for path, asset := range map[string]string{WorkflowPath: workflowAsset, ReconcilerPath: reconcilerAsset} {
		body, err := fs.ReadFile(Assets, asset)
		if err != nil {
			return done, err
		}
		if err := write(root, path, body, &done); err != nil {
			return done, err
		}
	}
	if err := os.Remove(filepath.Join(root, supersededPath)); err == nil {
		done = append(done, "removed "+supersededPath)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return done, err
	}
	sort.Strings(done)
	return done, nil
}

func write(root, rel string, body []byte, done *[]string) error {
	path := filepath.Join(root, rel)
	old, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(old, body):
		return nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	}
	verb := "updated "
	if err != nil {
		verb = "created "
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return err
	}
	*done = append(*done, verb+rel)
	return nil
}

func ensureLabels(forge Forge, mapped []string, done *[]string) error {
	out, err := forge("label", "list", "--limit", "1000", "--json", "name")
	if err != nil {
		return err
	}
	var have []struct{ Name string }
	if err := json.Unmarshal(out, &have); err != nil {
		return fmt.Errorf("reading labels: %w", err)
	}
	exists := map[string]bool{}
	for _, l := range have {
		exists[l.Name] = true
	}
	for _, name := range mapped {
		if exists[name] {
			continue
		}
		if _, err := forge("label", "create", name, "--force"); err != nil {
			return err
		}
		exists[name] = true
		*done = append(*done, "created label "+name)
	}
	return nil
}
