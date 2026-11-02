package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

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

// FolderHash is the folder hash a lock entry's computedHash records, with the forward-slash paths of the files it covered, for callers outside this package such as change-map. A path the hash skipped (under .git or node_modules, or not a regular file) is not in the list, so no caller can treat it as verified.
func FolderHash(dir string) (string, []string, error) { return hashFolder(dir) }

// folderHash reimplements skills.sh's computeSkillFolderHash: sha256 over each regular file's forward-slash path then its bytes, files ordered by localeCompare, .git and node_modules skipped.
func folderHash(dir string) (string, error) {
	hash, _, err := hashFolder(dir)
	return hash, err
}

func hashFolder(dir string) (string, []string, error) {
	var rels []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	c := collate.New(language.Und)
	sort.SliceStable(rels, func(i, j int) bool { return c.CompareString(rels[i], rels[j]) < 0 })
	h := sha256.New()
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", nil, err
		}
		h.Write([]byte(rel))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), rels, nil
}
