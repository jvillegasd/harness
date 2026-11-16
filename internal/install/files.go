package install

import (
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
	assigned := choice.ForRender()
	catalog := new(models.Catalog)
	for _, role := range models.AllRoles {
		a := assigned[role]
		err := catalog.Runnable(models.Assignment{Client: a.Client, Model: a.Model, Effort: a.Effort})

		quiet := a.Source == "reviewer"
		switch {
		case err != nil && quiet:
			delete(assigned, role)
		case errors.Is(err, models.ErrNotListed):
			in.warn("%s is assigned %s model %s, which this machine does not list (%v); its agent keeps the client default until `harness models` names one this machine can run.", role, a.Client, a.Model, err)
			delete(assigned, role)
		case err != nil:
			in.warn("%v; %s keeps the %s client default.", err, role, a.Client)
			delete(assigned, role)
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
		if code := agents.Generate(in.dir.Root, in.agentOut(), in.assigned(), in.stdout, in.stderr); code != 0 {
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
			if current != "" && current != recorded[e.Path] {
				in.backupExisting(e.Path, "")
			}
			copyManaged(e.source, e.Path)
			in.log("Installed %s", e.Path)
		}
		want[i].Digest = source
	}

	in.updateState(func(s *state) { s.Files = append(kept, want...) })
	in.installed = want
}

func (in *installer) checkFiles() {
	for _, e := range in.installed {
		if digest(e.Path) != e.Digest {
			die("Managed copy missing or changed: %s", e.Path)
		}
	}
	if !in.has("agents") {
		return
	}
	if code := agents.Check(in.dir.Root, in.agentOut(), in.assigned(), in.stdout, in.stderr); code != 0 {
		die("Role agents in %s are missing, drifted, or widened.", in.agentOut())
	}
}
