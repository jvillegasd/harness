package skills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jvillegasd/harness/internal/checkout"
)

// LockResult is one locked skill's offline verification. Status is match, drift, or unverifiable; Detail carries the two hashes for drift.
type LockResult struct {
	Name   string
	Status string
	Detail string
}

// Verify recomputes every locked body's folder hash offline and reports each skill's status in lock order. It reads only and never rewrites a lock.
func Verify(c checkout.Checkout) ([]LockResult, error) {
	p := pathsFor(c)
	lf, err := readLock(p.lock)
	if err != nil {
		return nil, err
	}
	var results []LockResult
	for _, name := range lf.skills.keys {
		var e entry
		if err := json.Unmarshal(lf.skills.values[name], &e); err != nil || !isLocalHash(e.ComputedHash) {
			results = append(results, LockResult{Name: name, Status: "unverifiable"})
			continue
		}
		body, err := findBody(p, name)
		if err != nil || body == "" {
			results = append(results, LockResult{Name: name, Status: "unverifiable"})
			continue
		}
		got, err := folderHash(body)
		if err != nil {
			results = append(results, LockResult{Name: name, Status: "unverifiable"})
			continue
		}
		if got == e.ComputedHash {
			results = append(results, LockResult{Name: name, Status: "match"})
			continue
		}
		results = append(results, LockResult{Name: name, Status: "drift", Detail: fmt.Sprintf("recorded %s, computed %s", e.ComputedHash, got)})
	}
	return results, nil
}

func verify(c checkout.Checkout, stdout, stderr io.Writer) error {
	results, err := Verify(c)
	if err != nil {
		return err
	}
	var drifted, unverifiable int
	for _, r := range results {
		switch r.Status {
		case "match":
			fmt.Fprintf(stdout, "%s: match\n", r.Name)
		case "drift":
			fmt.Fprintf(stdout, "%s: drift (%s)\n", r.Name, r.Detail)
			drifted++
		default:
			fmt.Fprintf(stdout, "%s: unverifiable\n", r.Name)
			unverifiable++
		}
	}
	if unverifiable > 0 {
		fmt.Fprintf(stderr, "harness skills: %d skill(s) unverifiable\n", unverifiable)
	}
	unguarded, err := Unguarded(c)
	if err != nil {
		return err
	}
	for _, problem := range unguarded {
		fmt.Fprintln(stdout, problem)
	}
	if drifted > 0 || len(unguarded) > 0 {
		return fmt.Errorf("%d skill(s) drifted, %d invocation guard(s) missing", drifted, len(unguarded))
	}
	return nil
}

// isLocalHash reports whether s is the bare lowercase sha256 hex skills.sh records for a folder it hashed locally. A snapshot hash from a blob install, such as a git tree sha, is not, so verify calls it unverifiable rather than drift.
func isLocalHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// FolderHash is the folder hash a lock entry's computedHash records, with the forward-slash paths of the files it covered, for callers outside this package such as change-map. A path the hash skipped is not in the list, so no caller can treat it as verified.
func FolderHash(dir string) (string, []string, error) { return hashFolder(dir) }

// folderHash reimplements the computeSkillFolderHash of skills.sh's version 2 lock: sha256 over each file's forward-slash path, NUL, its length, NUL, and its bytes, files in byte order of path. Text files hash with CRLF folded to LF. metadata.json files and .git, __pycache__, and __pypackages__ directories are skipped, and a symlink is followed only while it resolves inside the folder.
func folderHash(dir string) (string, error) {
	hash, _, err := hashFolder(dir)
	return hash, err
}

type hashedFile struct {
	path    string
	content []byte
}

func hashFolder(dir string) (string, []string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", nil, err
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", nil, err
	}
	var files []hashedFile
	if err := collectHashed(dir, "", root, map[string]bool{}, &files); err != nil {
		return "", nil, err
	}
	for i := range files {
		files[i].path = strings.ReplaceAll(files[i].path, "\\", "/")
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].path < files[j].path })
	h := sha256.New()
	rels := make([]string, 0, len(files))
	for _, f := range files {
		content := f.content
		if bytes.IndexByte(content, 0) < 0 && bytes.Contains(content, []byte("\r\n")) {
			content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
		}
		h.Write([]byte(f.path))
		h.Write([]byte{0})
		h.Write([]byte(strconv.Itoa(len(content))))
		h.Write([]byte{0})
		h.Write(content)
		rels = append(rels, f.path)
	}
	return hex.EncodeToString(h.Sum(nil)), rels, nil
}

// collectHashed walks dir the way skills.sh's collectFiles does, tracking the real directories above it so a symlink loop ends instead of recursing.
func collectHashed(dir, prefix, root string, ancestors map[string]bool, files *[]hashedFile) error {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	if ancestors[real] {
		return nil
	}
	branch := map[string]bool{real: true}
	for a := range ancestors {
		branch[a] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		isDir := e.IsDir()
		if excludedFromHash(name, isDir) {
			continue
		}
		full := filepath.Join(dir, name)
		if e.Type()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(full)
			if err != nil || (target != root && !strings.HasPrefix(target, root+string(filepath.Separator))) {
				continue
			}
			fi, err := os.Stat(target)
			if err != nil {
				return err
			}
			isDir = fi.IsDir()
			if isDir && excludedFromHash(name, true) {
				continue
			}
		} else if !isDir && !e.Type().IsRegular() {
			continue
		}
		if isDir {
			if err := collectHashed(full, prefix+name+"/", root, branch, files); err != nil {
				return err
			}
			continue
		}
		content, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		*files = append(*files, hashedFile{path: prefix + name, content: content})
	}
	return nil
}

func excludedFromHash(name string, isDir bool) bool {
	if name == "metadata.json" {
		return true
	}
	return isDir && (name == ".git" || name == "__pycache__" || name == "__pypackages__")
}
