package eval

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	mrand "math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const RootEnv = "HARNESS_SCRATCH_ROOT"

var Banned = []string{"eval", "test", "judge", "rubric", "score", "benchmark", "candidate", "compare"}

var projectWords = []string{"orchard", "harbor", "copper", "meadow", "lantern", "summit", "willow", "granite", "beacon", "cedar", "juniper", "marble", "pebble", "quarry", "saffron", "tundra", "violet", "walnut", "maple", "ember"}

var skillPaths = map[string]string{
	"claude":   filepath.Join(".claude", "skills"),
	"codex":    filepath.Join(".agents", "skills"),
	"opencode": filepath.Join(".agents", "skills"),
}

const opencodeConfig = "{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"permission\": {\n    \"external_directory\": \"deny\"\n  }\n}\n"

const (
	Variant  = "variant"
	Baseline = "baseline"
)

type Planted struct {
	Label string `json:"label"`
	Dir   string `json:"dir"`
}

type Manifest struct {
	Run    string    `json:"run"`
	Client string    `json:"client"`
	Skill  string    `json:"skill"`
	Root   string    `json:"root"`
	Dirs   []Planted `json:"dirs"`
}

type Key struct {
	Run    string            `json:"run"`
	Labels map[string]string `json:"labels"`
}

func Root(home string) string {
	if dir := os.Getenv(RootEnv); dir != "" {
		return dir
	}
	return filepath.Join(home, ".scratch-projects")
}

func MetaDir(home, run string) string { return filepath.Join(home, ".harness", "eval", run) }

var (
	runIDRe     = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{6}$`)
	skillNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

func ValidRun(run string) bool { return runIDRe.MatchString(run) }

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func skillName(dir string) (string, error) {
	f, err := os.Open(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return "", fmt.Errorf("%s holds no SKILL.md", dir)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if !s.Scan() || strings.TrimSpace(s.Text()) != "---" {
		return "", fmt.Errorf("%s/SKILL.md has no frontmatter", dir)
	}
	for s.Scan() {
		line := s.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		if v, ok := strings.CutPrefix(line, "name:"); ok {
			if name := strings.Trim(strings.TrimSpace(v), `"'`); name != "" {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("%s/SKILL.md names no skill", dir)
}

type PlantOptions struct {
	Variant, Baseline, Skeleton string
	Client                      string
	Runs                        int
}

func Plant(home string, o PlantOptions) (Manifest, error) {
	m := Manifest{Client: o.Client, Root: Root(home)}
	skillDir, ok := skillPaths[o.Client]
	if !ok {
		return m, fmt.Errorf("unknown client %q; use claude, codex, or opencode", o.Client)
	}
	if o.Runs < 1 {
		return m, errors.New("--runs must be at least 1")
	}
	name, err := skillName(o.Variant)
	if err != nil {
		return m, err
	}
	if !skillNameRe.MatchString(name) {
		return m, fmt.Errorf("invalid skill name %q", name)
	}
	for _, word := range Banned {
		if strings.Contains(strings.ToLower(name), word) {
			return m, fmt.Errorf("skill name %q contains a banned word", name)
		}
	}
	if base, err := skillName(o.Baseline); err != nil {
		return m, err
	} else if base != name {
		return m, fmt.Errorf("the variant is skill %s and the baseline %s; compare two versions of one skill", name, base)
	}
	if o.Skeleton != "" {
		if fi, err := os.Stat(o.Skeleton); err != nil || !fi.IsDir() {
			return m, fmt.Errorf("--skeleton %s is not a directory", o.Skeleton)
		}
	}
	m.Skill = name
	m.Run = time.Now().UTC().Format("20060102-150405") + "-" + randomHex(3)
	if err := os.MkdirAll(m.Root, 0o755); err != nil {
		return m, err
	}
	arms := make([]string, 0, 2*o.Runs)
	for range o.Runs {
		arms = append(arms, Variant, Baseline)
	}
	mrand.Shuffle(len(arms), func(i, j int) { arms[i], arms[j] = arms[j], arms[i] })
	key := Key{Run: m.Run, Labels: map[string]string{}}
	for i, arm := range arms {
		label := fmt.Sprintf("r%d", i+1)
		key.Labels[label] = arm
		dir, err := freshDir(m.Root)
		if err != nil {
			return m, err
		}
		m.Dirs = append(m.Dirs, Planted{label, dir})
	}
	meta := MetaDir(home, m.Run)
	if err := os.MkdirAll(meta, 0o700); err != nil {
		return m, err
	}
	if err := writeJSON(filepath.Join(meta, "manifest.json"), m); err != nil {
		return m, err
	}
	if err := writeJSON(filepath.Join(meta, "key.json"), key); err != nil {
		return m, err
	}
	for _, p := range m.Dirs {
		if err := os.Mkdir(p.Dir, 0o755); err != nil {
			return m, err
		}
		if o.Skeleton != "" {
			if err := copyTree(o.Skeleton, p.Dir); err != nil {
				return m, err
			}
		}
		source := o.Variant
		if key.Labels[p.Label] == Baseline {
			source = o.Baseline
		}
		if err := copyTree(source, filepath.Join(p.Dir, skillDir, name)); err != nil {
			return m, err
		}
		if o.Client == "opencode" {
			config := filepath.Join(p.Dir, "opencode.json")
			if _, err := os.Stat(config); errors.Is(err, fs.ErrNotExist) {
				if err := os.WriteFile(config, []byte(opencodeConfig), 0o644); err != nil {
					return m, err
				}
			}
		}
	}
	return m, nil
}

func freshDir(root string) (string, error) {
	for range 100 {
		dir := filepath.Join(root, projectWords[mrand.IntN(len(projectWords))]+"-"+randomHex(2))
		if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
			return dir, nil
		}
	}
	return "", fmt.Errorf("no free directory name under %s", root)
}

func ReadRun(home, run string) (Manifest, Key, error) {
	var m Manifest
	var k Key
	if !ValidRun(run) {
		return m, k, fmt.Errorf("%q is not a run id", run)
	}
	meta := MetaDir(home, run)
	raw, err := os.ReadFile(filepath.Join(meta, "manifest.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return m, k, fmt.Errorf("no run %s", run)
	}
	if err != nil {
		return m, k, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, k, fmt.Errorf("%s: %w", run, err)
	}
	raw, err = os.ReadFile(filepath.Join(meta, "key.json"))
	if err != nil {
		return m, k, err
	}
	return m, k, json.Unmarshal(raw, &k)
}

func Clean(home, run string) ([]string, error) {
	m, _, err := ReadRun(home, run)
	if err != nil {
		return nil, err
	}
	for _, p := range m.Dirs {
		rel, err := filepath.Rel(m.Root, p.Dir)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || strings.ContainsRune(rel, filepath.Separator) {
			return nil, fmt.Errorf("refusing to remove %s, which is not a directory directly under %s", p.Dir, m.Root)
		}
	}
	var removed []string
	for _, p := range m.Dirs {
		if _, err := os.Lstat(p.Dir); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := os.RemoveAll(p.Dir); err != nil {
			return removed, err
		}
		removed = append(removed, p.Dir)
	}
	meta := MetaDir(home, run)
	if err := os.RemoveAll(meta); err != nil {
		return removed, err
	}
	return append(removed, meta), nil
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
