package install

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jvillegasd/harness/internal/agents"
	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/models"
)

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
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

func (in *installer) assigned() map[string]agents.Assigned {

	if in.assignments != nil {
		return in.assignments
	}
	choice, err := models.Load(models.StatePath(in.home))
	if err != nil {
		die("Reading the model choice failed: %v", err)
	}
	assigned, all := choice.ForRender(new(models.Catalog))
	for _, role := range models.AllRoles {
		r := all[role]
		switch {
		case r.Err != nil && r.Source == "reviewer":
		case errors.Is(r.Err, models.ErrNotListed):
			in.warnFor("agents", "%s runs at the %s tier on %s, which this machine cannot serve (%v); its agent keeps the client default until `harness models` names a model this machine can run.", role, r.Tier, r.Client, r.Err)
		case r.Err != nil:
			in.warnFor("agents", "%v; %s keeps the %s client default.", r.Err, role, r.Client)
		case r.Note != "" && r.Source != "reviewer":
			in.warnFor("agents", "%s: %s.", role, r.Note)
		}
	}
	in.assignments = assigned
	return assigned
}

func (in *installer) agentOut() string {
	return filepath.Join(in.dir.Generated(), "agents")
}

func (in *installer) agentRoots() [][2]string {
	return [][2]string{{"claude", filepath.Join(in.home, ".claude", "agents")}, {"codex", filepath.Join(in.codexHome(), "agents")}, {"opencode", filepath.Join(in.config, "opencode", "agents")}}
}

func (in *installer) skills() []checkout.Skill {
	skills, err := in.dir.SkillDirs()
	if err != nil {
		die("%v", err)
	}
	return skills
}

func (in *installer) ensureBackupRoot() {
	if in.backupRoot == "" {
		in.backupRoot = filepath.Join(in.home, ".dotfiles-backups", "harness", fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), os.Getpid()))
		must(os.MkdirAll(in.backupRoot, 0o755))
	}
}

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

func (in *installer) inScope(e entry) bool {
	return in.has(e.Component) && slices.ContainsFunc(e.Clients, in.targets)
}

func (in *installer) installFiles() {
	if in.has("skills") {
		for _, root := range []string{filepath.Join(in.home, ".agents", "skills"), filepath.Join(in.home, ".claude", "skills")} {
			in.ensureManagedDir(root, true)
		}

		in.ensureManagedDir(filepath.Join(in.config, "opencode", "skills"), false)
	}
	if in.has("agents") {
		for _, pair := range in.agentRoots() {
			if in.targets(pair[0]) {
				in.ensureManagedDir(pair[1], true)
			}
		}
		if code := agents.Generate(in.dir.Root, in.agentOut(), in.assigned(), io.Discard, in.stderr); code != 0 {
			die("Generating the role agents failed with status %d.", code)
		}
	}

	previous := in.loadState()
	recorded := map[string]string{}
	for _, e := range previous.Files {
		recorded[e.Path] = e.Digest
	}
	want := slices.DeleteFunc(in.desired(), func(e entry) bool { return !in.inScope(e) })
	wanted := map[string]bool{}
	for _, e := range want {
		wanted[e.Path] = true
	}
	var kept []entry
	for _, e := range previous.Files {
		switch {
		case wanted[e.Path]:
		case in.inScope(e):
			in.removeManaged(e)
		default:
			kept = append(kept, e)
		}
	}
	for i, e := range want {
		if e.Component == "skills" {

			in.backupExisting(filepath.Join(in.config, "opencode", "skills", filepath.Base(e.Path)), "")
		}
		current, source := digest(e.Path), digest(e.source)
		if current != source {
			edited := current != "" && current != recorded[e.Path]
			if edited && in.keepEdited {
				in.warnFor(e.Component, "%s changed since the harness wrote it, so it stays; `harness install agents` archives it and renders the choice.", e.Path)
				in.file(e.Component, "kept", e.Path)
				want[i].Digest = recorded[e.Path]
				if in.keptEdited == nil {
					in.keptEdited = map[string]bool{}
				}
				in.keptEdited[e.Path] = true
				continue
			}
			if edited {
				in.backupExisting(e.Path, "")
			}
			copyManaged(e.source, e.Path)
			in.file(e.Component, "installed", e.Path)
		} else {
			in.file(e.Component, "unchanged", e.Path)
		}
		want[i].Digest = source
	}

	in.updateState(func(s *state) { s.Files = append(kept, want...) })
	in.installed = want
}

func (in *installer) checkFiles() {
	for _, e := range in.installed {
		if !in.keptEdited[e.Path] && digest(e.Path) != e.Digest {
			die("Managed copy missing or changed: %s", e.Path)
		}
	}
	if !in.has("agents") {
		return
	}
	var findings strings.Builder
	if code := agents.Check(in.dir.Root, in.agentOut(), in.assigned(), &findings, in.stderr); code != 0 {
		for _, finding := range strings.Split(strings.TrimSpace(findings.String()), "\n") {
			in.warn("%s", finding)
		}
		die("Role agents in %s are missing, drifted, or widened.", in.agentOut())
	}
}
