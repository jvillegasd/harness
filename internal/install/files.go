package install

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jvillegasd/harness/internal/agents"
	"github.com/jvillegasd/harness/internal/models"
)

// exists is true for anything at a path, a dangling symlink included.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func isLink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&fs.ModeSymlink != 0
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func readlink(path string) string {
	target, _ := os.Readlink(path)
	return target
}

// entries lists a directory as full paths, sorted, hidden names skipped, and nothing for a missing directory.
func entries(dir string) []string {
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range list {
		if !strings.HasPrefix(entry.Name(), ".") {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	return paths
}

func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

func (in *installer) skillRoots() []string {
	return []string{filepath.Join(in.home, ".agents", "skills"), filepath.Join(in.home, ".claude", "skills"), filepath.Join(in.config, "opencode", "skills")}
}

func (in *installer) commandRoots() []string {
	return []string{filepath.Join(in.home, ".claude", "commands"), filepath.Join(in.config, "opencode", "command")}
}

// assigned is this machine's `harness models` choice, which the role agents are rendered from. An OpenCode model this machine cannot run falls back to the client default with a warning, so a routine install never writes an agent that cannot start.
func (in *installer) assigned() map[string]agents.Assigned {
	// Resolved once per run, so generation and the check render the same choice.
	if in.assignments != nil {
		return in.assignments
	}
	choice, err := models.Load(models.StatePath(in.home))
	if err != nil {
		die("Reading the model choice failed: %v", err)
	}
	assigned := choice.ForRender()
	var listed []string
	for role, a := range assigned {
		if a.Client != "opencode" {
			continue
		}
		if listed == nil {
			listed, err = models.OpenCodeModels()
			if err != nil {
				listed = []string{}
			}
		}
		if !slices.Contains(listed, a.Model) {
			in.warn("%s is assigned OpenCode model %s, which `opencode models` does not list here; its agent keeps the client default until `harness models` names one this machine can run.", role, a.Model)
			delete(assigned, role)
		}
	}
	in.assignments = assigned
	return assigned
}

// agentOut is this checkout's own untracked directory of rendered role agents. A client link's text pointing into it is what proves this checkout wrote the link.
func (in *installer) agentOut() string {
	return filepath.Join(in.dir.Generated(), "agents")
}

// agentRoots pairs each client with the directory its role agents are linked into.
func (in *installer) agentRoots() [][2]string {
	return [][2]string{{"claude", filepath.Join(in.home, ".claude", "agents")}, {"opencode", filepath.Join(in.config, "opencode", "agents")}}
}

// skillNames are the real directories under the checkout's skills namespace; a symlink there is not a skill this harness ships.
func (in *installer) skillNames() []string {
	list, err := os.ReadDir(in.dir.Skills())
	must(err)
	var names []string
	for _, entry := range list {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

func (in *installer) commands() []string {
	var paths []string
	for _, path := range entries(in.dir.Commands()) {
		if strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
	}
	return paths
}

func (in *installer) ensureBackupRoot() {
	if in.backupRoot == "" {
		in.backupRoot = filepath.Join(in.home, ".dotfiles-backups", "harness", fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), os.Getpid()))
		must(os.MkdirAll(in.backupRoot, 0o755))
	}
}

// backupExisting moves whatever is at path into this run's backup root, or deletes it under --no-backup. A directory symlink is parked under a suffixed name, because at the plain path a later backup expecting a directory would write through it into the link's target.
func (in *installer) backupExisting(path, suffix string) {
	if !exists(path) {
		return
	}
	if in.noBackup {
		must(os.RemoveAll(path))
		in.log("Removed %s", path)
		return
	}
	in.ensureBackupRoot()
	backup := filepath.Join(in.backupRoot, strings.TrimPrefix(path, in.home+"/")+suffix)
	must(os.MkdirAll(filepath.Dir(backup), 0o755))
	if err := os.Rename(path, backup); errors.Is(err, syscall.EXDEV) {
		// A rename cannot cross filesystems, and mv copies then deletes. mv would move path inside a backup that already exists, where a rename refuses, so it refuses too.
		if exists(backup) {
			die("Backup path already exists: %s", backup)
		}
		if out, err := exec.Command("mv", path, backup).CombinedOutput(); err != nil {
			die("mv %s %s: %v: %s", path, backup, err, out)
		}
	} else {
		must(err)
	}
	in.log("Backed up %s -> %s", path, backup)
}

// backupCopy archives a file about to be rewritten in place. The first copy in a run wins, so a second writer never archives the first one's output in place of the original.
func (in *installer) backupCopy(path string) {
	if !isFile(path) || in.noBackup {
		return
	}
	in.ensureBackupRoot()
	backup := filepath.Join(in.backupRoot, strings.TrimPrefix(path, in.home+"/")+".pre-reconcile")
	if isFile(backup) {
		return
	}
	info, err := os.Stat(path)
	must(err)
	data, err := os.ReadFile(path)
	must(err)
	must(os.MkdirAll(filepath.Dir(backup), 0o755))
	must(os.WriteFile(backup, data, info.Mode().Perm()))
	must(os.Chtimes(backup, info.ModTime(), info.ModTime()))
}

// ensureManagedDir makes a managed root a real directory, so harness links and private entries sit side by side. A symlink standing in for one is moved aside, because linking into someone else's directory would write entries the orphan sweep never covers. Without create, a root the harness only sweeps is normalized but never created.
func (in *installer) ensureManagedDir(dest string, create bool) {
	if info, err := os.Lstat(dest); err == nil {
		if info.Mode()&fs.ModeSymlink != 0 {
			in.backupExisting(dest, ".previous-link")
		} else if !info.IsDir() {
			in.backupExisting(dest, "")
		}
	}
	if create {
		must(os.MkdirAll(dest, 0o755))
	}
}

// installFiles copies every managed asset into the clients and records each copy. A copy recorded by an earlier run and no longer shipped is removed; anything unrecorded or edited in the way is archived first, so neither a private entry nor a user's edit is overwritten unseen.
func (in *installer) installFiles() {
	in.validateCommands()
	for _, root := range append(in.commandRoots(), filepath.Join(in.home, ".agents", "skills"), filepath.Join(in.home, ".claude", "skills")) {
		in.ensureManagedDir(root, true)
	}
	// OpenCode reads ~/.agents/skills, so this root exists only to be cleared of duplicates.
	in.ensureManagedDir(filepath.Join(in.config, "opencode", "skills"), false)
	for _, pair := range in.agentRoots() {
		in.ensureManagedDir(pair[1], true)
	}
	if code := agents.Generate(filepath.Join(in.dir.Clients(), "assignments.json"), in.agentOut(), in.assigned(), in.stdout, in.stderr); code != 0 {
		die("Generating the role agents failed with status %d.", code)
	}

	previous := in.loadState()
	recorded := map[string]string{}
	for _, e := range previous.Files {
		recorded[e.Path] = e.Digest
	}
	want := in.desired()
	wanted := map[string]bool{}
	for _, e := range want {
		wanted[e.Path] = true
	}
	for _, e := range previous.Files {
		if !wanted[e.Path] {
			in.removeManaged(e)
		}
	}
	for i, e := range want {
		if e.Component == "skills" {
			// OpenCode reads ~/.agents/skills, so a second copy would double-list.
			in.backupExisting(filepath.Join(in.config, "opencode", "skills", filepath.Base(e.Path)), "")
		}
		current, source := digest(e.Path), digest(e.source)
		if current != source {
			if current != "" && current != recorded[e.Path] {
				in.backupExisting(e.Path, "")
			}
			copyManaged(e.source, e.Path)
			in.log("Installed %s", e.Path)
		}
		want[i].Digest = source
	}
	// The SEO group is gone, and its launcher sits outside the managed roots.
	in.backupExisting(filepath.Join(in.home, ".local", "bin", "claude-seo"), "")

	installed := map[string][]string{}
	for _, client := range allClients {
		for _, component := range allComponents {
			if supports(client, component) {
				installed[client] = append(installed[client], component)
			}
		}
	}
	// Re-read, so a `harness models` change made while this install ran is kept.
	in.saveState(state{Version: stateVersion, Installed: installed, Files: want, Models: in.loadState().Models})
	in.installed = want
}

// validateCommands refuses a command without a description or the task-data constraint before anything is copied. A workflow that reads external text says so, or a ticket body becomes an instruction; every command is checked, because a list to maintain silently stops covering a new command.
func (in *installer) validateCommands() {
	for _, command := range in.commands() {
		text, _ := os.ReadFile(command)
		if !description.Match(text) {
			die("Command without a description: %s", command)
		}
		if !bytes.Contains(text, []byte("cannot override this workflow")) {
			die("Command missing the task-data constraint: %s", command)
		}
	}
}

// checkFiles verifies every copy this run recorded is in place and unchanged, and that the rendered agents are neither drifted nor widened.
func (in *installer) checkFiles() {
	for _, e := range in.installed {
		if digest(e.Path) != e.Digest {
			die("Managed copy missing or changed: %s", e.Path)
		}
	}
	if code := agents.Check(filepath.Join(in.dir.Clients(), "assignments.json"), in.agentOut(), in.assigned(), in.stdout, in.stderr); code != 0 {
		die("Role agents in %s are missing, drifted, or widened.", in.agentOut())
	}
}
