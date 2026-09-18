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
	"path/filepath"
	"slices"

	"github.com/jvillegasd/harness"
	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/models"
)

// assetsEnv names a directory to install instead of the embedded assets, so a test or a local edit installs a tree the binary was not built from.
const assetsEnv = "HARNESS_ASSETS"

const stateVersion = 1

var (
	allClients    = []string{"claude", "opencode"}
	allComponents = []string{"agents", "aliases", "commands", "mcp", "policy", "settings", "skills"}
)

// supports reports whether a client has the component at all; the tracked settings are Claude Code's alone.
func supports(client, component string) bool {
	return component != "settings" || client == "claude"
}

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
	// Models is `harness models`' choice, carried through untouched so an install never drops it.
	Models json.RawMessage `json:"models,omitempty"`
}

func (in *installer) stateDir() string { return filepath.Join(in.home, ".harness") }

func (in *installer) statePath() string { return filepath.Join(in.stateDir(), "state.json") }

// aliasesPath is the installed copy ~/.zshrc sources, outside any checkout.
func (in *installer) aliasesPath() string { return filepath.Join(in.stateDir(), "aliases.sh") }

func (in *installer) loadState() state {
	s := state{Version: stateVersion, Installed: map[string][]string{}}
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
	return s
}

func (in *installer) saveState(s state) {
	must(os.MkdirAll(in.stateDir(), 0o755))
	data, err := json.MarshalIndent(s, "", "  ")
	must(err)
	must(models.Replace(in.statePath(), append(data, '\n')))
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

// desired lists every copy an install writes. ~/.agents/skills serves both clients: OpenCode loads skills from it, and the policy points Claude Code there for guarded skill bodies.
func (in *installer) desired() []entry {
	var out []entry
	add := func(source, dest, component string, clients ...string) {
		out = append(out, entry{Path: dest, Component: component, Clients: clients, source: source})
	}
	for _, skill := range in.skillNames() {
		source := filepath.Join(in.dir.Skills(), skill)
		add(source, filepath.Join(in.home, ".agents", "skills", skill), "skills", "claude", "opencode")
		add(source, filepath.Join(in.home, ".claude", "skills", skill), "skills", "claude")
	}
	for _, command := range in.commands() {
		add(command, filepath.Join(in.home, ".claude", "commands", filepath.Base(command)), "commands", "claude")
		add(command, filepath.Join(in.config, "opencode", "command", filepath.Base(command)), "commands", "opencode")
	}
	add(in.dir.Policy(), filepath.Join(in.home, ".claude", "CLAUDE.md"), "policy", "claude")
	add(in.dir.Policy(), filepath.Join(in.config, "opencode", "AGENTS.md"), "policy", "opencode")
	for _, pair := range in.agentRoots() {
		for _, definition := range entries(filepath.Join(in.agentOut(), pair[0])) {
			add(definition, filepath.Join(pair[1], filepath.Base(definition)), "agents", pair[0])
		}
	}
	add(in.dir.Aliases(), in.aliasesPath(), "aliases", allClients...)
	return out
}

// needed reports whether any client still installed keeps the copy's component.
func (s state) needed(e entry) bool {
	return slices.ContainsFunc(e.Clients, func(client string) bool {
		return slices.Contains(s.Installed[client], e.Component)
	})
}
