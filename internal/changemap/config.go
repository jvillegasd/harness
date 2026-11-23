package changemap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jvillegasd/harness/internal/skills"
)

const ConfigPath = "docs/agents/harness.json"

type Config struct {
	CriticalPaths []string

	Locked []Lock
}

const SkillLockPath = "skills-lock.json"

type Lock struct {
	Dir, Name, Source string
	Verified          bool

	Hashed map[string]bool
}

func LoadLocks(root string) ([]Lock, error) {
	raw, err := os.ReadFile(filepath.Join(root, SkillLockPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Skills map[string]struct {
			Source       string `json:"source"`
			SkillPath    string `json:"skillPath"`
			ComputedHash string `json:"computedHash"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %v", SkillLockPath, err)
	}
	var locks []Lock
	for name, entry := range doc.Skills {
		dir := path.Dir(entry.SkillPath)
		if entry.SkillPath == "" || dir == "." || dir == "/" || strings.HasPrefix(dir, "..") {
			return nil, fmt.Errorf("%s: skill %q has no usable skillPath %q", SkillLockPath, name, entry.SkillPath)
		}
		if !strings.HasPrefix(dir, "skills/") {
			continue
		}
		dir = bodyDir(root, name, dir)
		got, files, err := skills.FolderHash(filepath.Join(root, filepath.FromSlash(dir)))
		verified := err == nil && entry.ComputedHash != "" && got == entry.ComputedHash
		hashed := map[string]bool{}
		for _, file := range files {
			hashed[file] = true
		}
		locks = append(locks, Lock{Dir: dir, Name: name, Source: entry.Source, Verified: verified, Hashed: hashed})
	}
	sort.Slice(locks, func(i, j int) bool { return locks[i].Dir < locks[j].Dir })
	return locks, nil
}

func bodyDir(root, name, recorded string) string {
	candidates := []string{recorded, "skills/" + name}
	if groups, err := os.ReadDir(filepath.Join(root, "skills")); err == nil {
		for _, g := range groups {
			if g.IsDir() && !strings.HasPrefix(g.Name(), ".") {
				candidates = append(candidates, "skills/"+g.Name()+"/"+name)
			}
		}
	}
	for _, dir := range candidates {
		if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); err == nil && fi.IsDir() {
			return dir
		}
	}
	return recorded
}

func LoadConfig(root string) (Config, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ConfigPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	return parseConfig(raw, ConfigPath)
}

func EffectiveConfig(root, mergeBase string) (Config, error) {
	current, err := LoadConfig(root)
	if err != nil {
		return Config{}, err
	}
	revs := []string{mergeBase}
	if def, err := git(root, "merge-base", "HEAD", DefaultBase(root)); err == nil {
		revs = append(revs, strings.TrimSpace(def))
	}
	cfg := Config{}
	seen := map[string]bool{}
	add := func(globs []string) {
		for _, g := range globs {
			if !seen[g] {
				seen[g] = true
				cfg.CriticalPaths = append(cfg.CriticalPaths, g)
			}
		}
	}
	for _, rev := range revs {
		if rev == "" || seen["rev:"+rev] {
			continue
		}
		seen["rev:"+rev] = true
		base, err := configAt(root, rev)
		if err != nil {
			return Config{}, err
		}
		add(base.CriticalPaths)
	}
	add(current.CriticalPaths)
	return cfg, nil
}

func configAt(root, rev string) (Config, error) {
	if _, err := git(root, "cat-file", "-e", rev+":"+ConfigPath); err != nil {
		return Config{}, nil
	}
	raw, err := git(root, "show", rev+":"+ConfigPath)
	if err != nil {
		return Config{}, err
	}
	cfg, err := parseConfig([]byte(raw), ConfigPath+" at "+rev[:min(12, len(rev))])
	if err != nil {
		return Config{}, fmt.Errorf("%v; fix it on the default branch, because the committed base config still sets the floor", err)
	}
	return cfg, nil
}

func parseConfig(raw []byte, source string) (Config, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Config{}, fmt.Errorf("%s: %v", source, err)
	}
	field, ok := doc["criticalPaths"]
	if !ok {
		return Config{}, nil
	}
	if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
		return Config{}, fmt.Errorf("%s: criticalPaths must be an array of glob strings, got null", source)
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(field, &entries); err != nil {
		return Config{}, fmt.Errorf("%s: criticalPaths must be an array of glob strings: %v", source, err)
	}
	var cfg Config
	for i, entry := range entries {
		var glob string
		if err := json.Unmarshal(entry, &glob); err != nil {
			return Config{}, fmt.Errorf("%s: criticalPaths[%d] %s is not a string", source, i, entry)
		}
		if err := ValidateGlob(glob); err != nil {
			return Config{}, fmt.Errorf("%s: criticalPaths[%d] %q: %v", source, i, glob, err)
		}
		cfg.CriticalPaths = append(cfg.CriticalPaths, glob)
	}
	return cfg, nil
}
