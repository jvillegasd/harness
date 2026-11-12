package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/jvillegasd/harness"
	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/models"
	"github.com/jvillegasd/harness/internal/statefile"
)

// assetsEnv names a directory to install instead of the embedded assets, so a test or a local edit installs a tree the binary was not built from.
const assetsEnv = "HARNESS_ASSETS"

const stateVersion = 1

var (
	allClients = []string{"claude", "codex", "opencode"}
	// allComponents is every name install, update, and uninstall select by; configComponents are the ones that write managed config into a client. The tools table names the components that put tools on the machine, and mcps is both.
	allComponents    = []string{"clients", "packages", "mcps", "skills", "agents", "floor", "settings", "aliases"}
	configComponents = []string{"agents", "aliases", "floor", "mcps", "settings", "skills"}
)

// renamed maps a component name an older install recorded to its current one.
var renamed = map[string]string{"policy": "floor", "mcp": "mcps"}

// entry is one managed copy: where it was written, what it belongs to, and the digest it had when written, which is how a later run tells the harness's copy from a user's edit.
type entry struct {
	Path      string   `json:"path"`
	Component string   `json:"component"`
	Clients   []string `json:"clients"`
	Digest    string   `json:"sha256"`
	source    string
}

// state is ~/.harness/state.json: the components installed per client and every copy written for them. Sync and uninstall act on this record, never on what merely sits at a managed path.
type state struct {
	Version   int                 `json:"version"`
	Installed map[string][]string `json:"installed"`
	Files     []entry             `json:"files"`
	// Tools maps each tool this harness installed to how it installed it, which is the only record an uninstall removes a tool by. A tool present before the harness has no entry.
	Tools map[string]string `json:"tools,omitempty"`
	// Models is `harness models`' choice and Review is `harness review level`'s, both carried through untouched so an install never drops them.
	Models json.RawMessage `json:"models,omitempty"`
	Review json.RawMessage `json:"review,omitempty"`
}

// updateState applies change to the state on disk and writes it back at once, so the human's choices and every record another step wrote this run are kept.
func (in *installer) updateState(change func(*state)) {
	s := in.loadState()
	change(&s)
	in.saveState(s)
}

// empty reports whether the state records nothing the harness would still remove: no client keeps a component and no tool is recorded.
func (s state) empty() bool {
	return len(s.Installed) == 0 && len(s.Tools) == 0
}

// codexHome is ~/.codex whatever CODEX_HOME says, because a launcher may point CODEX_HOME at a home of its own in the terminals it opens.
func (in *installer) codexHome() string { return filepath.Join(in.home, ".codex") }

// codex runs a codex subcommand against codexHome, on the installer's output.
func (in *installer) codex(args ...string) *exec.Cmd {
	cmd := models.CodexCommand(in.home, args...)
	cmd.Stderr = in.stderr
	return cmd
}

func (in *installer) stateDir() string { return filepath.Join(in.home, ".harness") }

func (in *installer) statePath() string { return filepath.Join(in.stateDir(), "state.json") }

// aliasesPath is the installed copy the login shell's startup file sources, outside any checkout.
func (in *installer) aliasesPath() string { return filepath.Join(in.stateDir(), "aliases.sh") }

func (in *installer) loadState() state {
	s := state{Version: stateVersion, Installed: map[string][]string{}, Tools: map[string]string{}}
	data, err := os.ReadFile(in.statePath())
	if errors.Is(err, fs.ErrNotExist) {
		return s
	}
	must(err)
	if err := json.Unmarshal(data, &s); err != nil || s.Version != stateVersion {
		die("Unreadable harness state %s; fix or delete it and re-run.", in.statePath())
	}
	if s.Installed == nil {
		s.Installed = map[string][]string{}
	}
	if s.Tools == nil {
		s.Tools = map[string]string{}
	}
	// Older installs recorded policy and mcp; reading them under the current names migrates the file on its next write.
	for client, components := range s.Installed {
		for i, c := range components {
			if now, ok := renamed[c]; ok {
				components[i] = now
			}
		}
		s.Installed[client] = slices.Compact(sorted(components))
	}
	for i := range s.Files {
		if now, ok := renamed[s.Files[i].Component]; ok {
			s.Files[i].Component = now
		}
	}
	return s
}

func sorted(list []string) []string {
	out := slices.Clone(list)
	slices.Sort(out)
	return out
}

func (in *installer) saveState(s state) {
	must(os.MkdirAll(in.stateDir(), 0o755))
	data, err := json.MarshalIndent(s, "", "  ")
	must(err)
	must(statefile.Replace(in.statePath(), append(data, '\n')))
}

// extractAssets writes the assets this binary carries, or the HARNESS_ASSETS tree, into a temporary directory for this run to install from, and returns its removal.
func (in *installer) extractAssets() func() {
	var assets fs.FS = harness.Assets
	if dir := os.Getenv(assetsEnv); dir != "" {
		assets = os.DirFS(dir)
	}
	tmp, err := os.MkdirTemp("", "harness-assets-*")
	must(err)
	// The role agent renderer refuses a symlinked output path, and macOS TMPDIR sits under the /var symlink.
	if resolved, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = resolved
	}
	if err := os.CopyFS(tmp, assets); err != nil {
		_ = os.RemoveAll(tmp)
		die("Extracting the harness assets failed: %v", err)
	}
	in.dir = checkout.Checkout{Root: tmp}
	return func() { _ = os.RemoveAll(tmp) }
}

// digest hashes a file, or a directory's paths and contents in walk order, so an edit anywhere in a managed copy changes it. A missing path digests to "".
func digest(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	h := sha256.New()
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		fmt.Fprintf(h, "L %s\n", readlink(path))
	case !info.IsDir():
		data, err := os.ReadFile(path)
		must(err)
		h.Write(data)
	default:
		must(filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(path, p)
			switch {
			case d.Type()&fs.ModeSymlink != 0:
				fmt.Fprintf(h, "L %s %s\n", rel, readlink(p))
			case d.IsDir():
				fmt.Fprintf(h, "D %s\n", rel)
			default:
				data, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				fmt.Fprintf(h, "F %s %d\n", rel, len(data))
				h.Write(data)
			}
			return nil
		}))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// copyManaged writes source, a file or a whole directory, to dest. It builds beside dest and renames into place, so a client never reads a half-written copy.
func copyManaged(source, dest string) {
	must(os.MkdirAll(filepath.Dir(dest), 0o755))
	next := dest + ".harness-next"
	must(os.RemoveAll(next))
	info, err := os.Stat(source)
	must(err)
	if info.IsDir() {
		must(os.CopyFS(next, os.DirFS(source)))
	} else {
		src, err := os.Open(source)
		must(err)
		defer src.Close()
		dst, err := os.OpenFile(next, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		must(err)
		_, err = io.Copy(dst, src)
		must(errors.Join(err, dst.Close()))
	}
	must(os.RemoveAll(dest))
	must(os.Rename(next, dest))
}

// removeManaged deletes a recorded copy only while it still is what the harness wrote. An edited copy is the user's now and stays, with a warning.
func (in *installer) removeManaged(e entry) {
	current := digest(e.Path)
	if current == "" {
		return
	}
	if current != e.Digest {
		in.warn("%s changed since the harness wrote it, so it stays.", e.Path)
		return
	}
	must(os.RemoveAll(e.Path))
	in.log("Removed %s", e.Path)
}

// desired lists every copy an install writes. ~/.agents/skills serves every client: Codex and OpenCode load skills from it, and Claude Code finds the same bodies there.
func (in *installer) desired() []entry {
	var out []entry
	add := func(source, dest, component string, clients ...string) {
		out = append(out, entry{Path: dest, Component: component, Clients: clients, source: source})
	}
	for _, skill := range in.skills() {
		add(skill.Dir, filepath.Join(in.home, ".agents", "skills", skill.Name), "skills", "claude", "codex", "opencode")
		add(skill.Dir, filepath.Join(in.home, ".claude", "skills", skill.Name), "skills", "claude")
	}
	add(in.dir.Floor(), filepath.Join(in.home, ".claude", "CLAUDE.md"), "floor", "claude")
	add(in.dir.Floor(), filepath.Join(in.codexHome(), "AGENTS.md"), "floor", "codex")
	add(in.dir.Floor(), filepath.Join(in.config, "opencode", "AGENTS.md"), "floor", "opencode")
	for _, pair := range in.agentRoots() {
		for _, definition := range entries(filepath.Join(in.agentOut(), pair[0])) {
			add(definition, filepath.Join(pair[1], filepath.Base(definition)), "agents", pair[0])
		}
	}
	add(in.dir.Aliases(), in.aliasesPath(), "aliases", allClients...)
	// Claude Code's and Codex's compaction hooks are merged into files the user shares (reconcileClaudeSettings, reconcileCodexHooks); OpenCode's is a plugin file of its own.
	add(filepath.Join(in.dir.Clients(), "opencode", "plugins", "harness-resume.ts"), filepath.Join(in.config, "opencode", "plugins", "harness-resume.ts"), "settings", "opencode")
	// Codex loads every *.rules file under its rules directory, so the harness's command rules are a file of their own; OpenCode's live in opencode.json beside the user's (reconcileOpenCodePermission).
	add(filepath.Join(in.dir.Clients(), "codex", "rules", "harness.rules"), filepath.Join(in.codexHome(), "rules", "harness.rules"), "settings", "codex")
	return out
}

// needed reports whether any client still installed keeps the copy's component.
func (s state) needed(e entry) bool {
	return slices.ContainsFunc(e.Clients, func(client string) bool {
		return slices.Contains(s.Installed[client], e.Component)
	})
}
