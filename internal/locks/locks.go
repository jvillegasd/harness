// Package locks manages the reviewed third-party skill locks: the skills.sh calls behind add, update, and remove, the repository hygiene checks, and an offline verification of every locked body against its recorded folder hash.
package locks

import (
	"bytes"
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
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/jvillegasd/harness/internal/checkout"
)

const usage = `Usage: harness locks <command> [arguments]

Commands:
  add <owner/repo> <name>  Install and lock one upstream skill
  update                   Update every locked skill from upstream
  remove <name>            Remove one locked body, lock entry, and Claude link
  check                    Run shell syntax and diff hygiene checks
  verify                   Recompute every locked body's folder hash offline
`

// Command runs `harness locks` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "add", "update", "remove", "check", "verify":
	default:
		fmt.Fprintf(stderr, "harness locks: unknown command %q; run harness locks --help\n", args[0])
		return 2
	}
	c, err := checkout.Find(".")
	if err != nil {
		fmt.Fprintf(stderr, "[err] %v\n", err)
		return 1
	}
	var runErr error
	switch args[0] {
	case "add":
		runErr = add(c, args[1:], stdout, stderr)
	case "update":
		runErr = update(c, args[1:], stdout, stderr)
	case "remove":
		runErr = remove(c, args[1:], stdout, stderr)
	case "check":
		runErr = check(c, args[1:], stdout, stderr)
	case "verify":
		runErr = verify(c, args[1:], stdout, stderr)
	}
	if runErr != nil {
		fmt.Fprintf(stderr, "[err] %v\n", runErr)
		return 1
	}
	return 0
}

// paths are the checkout-relative locations the lock tool reads and writes.
type paths struct {
	root     string
	lock     string
	skills   string
	commands string
	claude   string
}

func pathsFor(c checkout.Checkout) paths {
	return paths{
		root:     c.Root,
		lock:     c.Lock(),
		skills:   c.Skills(),
		commands: c.Commands(),
		claude:   c.ClaudeSkills(),
	}
}

var (
	nameRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	ownerRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$`)
	repoRE  = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.-]*$`)
)

func validateName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid skill name: %s", name)
	}
	return nil
}

func validateSource(source string) error {
	owner, repo, ok := strings.Cut(source, "/")
	if !ok {
		return fmt.Errorf("source must be owner/repo: %s", source)
	}
	if !ownerRE.MatchString(owner) {
		return fmt.Errorf("invalid GitHub owner: %s", owner)
	}
	if !repoRE.MatchString(repo) || repo == "." || repo == ".." {
		return fmt.Errorf("invalid GitHub repository: %s", repo)
	}
	return nil
}

// orderedObject preserves a JSON object's keys and their raw values in file order, so a rewrite re-encodes only what it must and drops no field.
type orderedObject struct {
	keys   []string
	values map[string]json.RawMessage
}

func (o *orderedObject) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return errors.New("not a JSON object")
	}
	o.values = map[string]json.RawMessage{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return errors.New("non-string key")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		o.set(key, raw)
	}
	return nil
}

func (o *orderedObject) set(key string, raw json.RawMessage) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = raw
}

func (o *orderedObject) has(key string) bool {
	_, ok := o.values[key]
	return ok
}

func (o *orderedObject) del(key string) {
	if !o.has(key) {
		return
	}
	delete(o.values, key)
	keys := o.keys[:0]
	for _, k := range o.keys {
		if k != key {
			keys = append(keys, k)
		}
	}
	o.keys = keys
}

func (o *orderedObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeNoEscape(&buf, key); err != nil {
			return nil, err
		}
		buf.WriteByte(':')
		buf.Write(o.values[key])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// lockFile is the skills-lock.json document: an ordered top level plus the skills object parsed from it.
type lockFile struct {
	doc    orderedObject
	skills orderedObject
}

func (lf *lockFile) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &lf.doc); err != nil {
		return err
	}
	raw, ok := lf.doc.values["skills"]
	if !ok {
		raw = json.RawMessage("{}")
		lf.doc.set("skills", raw)
	}
	if err := json.Unmarshal(raw, &lf.skills); err != nil {
		return fmt.Errorf("skills: %w", err)
	}
	return nil
}

// MarshalJSON re-encodes only skills and keeps every other key and value raw, in file order, without HTML escaping, so writeLock stays byte-identical to skills.sh's JSON.stringify output.
func (lf *lockFile) MarshalJSON() ([]byte, error) {
	raw, err := lf.skills.MarshalJSON()
	if err != nil {
		return nil, err
	}
	lf.doc.set("skills", raw)
	return lf.doc.MarshalJSON()
}

// writeNoEscape encodes v without HTML escaping, matching JSON.stringify, and drops the newline Encode appends.
func writeNoEscape(buf *bytes.Buffer, v any) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	buf.Truncate(buf.Len() - 1)
	return nil
}

// entry is the subset of a lock entry verify reads.
type entry struct {
	ComputedHash string `json:"computedHash"`
}

func readLock(path string) (*lockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lf lockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &lf, nil
}

// writeLock reproduces skills.sh's canonical output: two-space indent, no HTML escaping, trailing newline, keys sorted.
func writeLock(path string, lf *lockFile) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(lf); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func run(root string, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func add(c checkout.Checkout, args []string, stdout, stderr io.Writer) error {
	if len(args) != 2 {
		return errors.New("add expects 2 argument(s)")
	}
	source, name := args[0], args[1]
	if err := validateSource(source); err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return err
	}
	p := pathsFor(c)
	lf, err := readLock(p.lock)
	if err != nil {
		return err
	}
	if lf.skills.has(name) {
		return fmt.Errorf("%s is already locked; use update", name)
	}
	body := filepath.Join(p.skills, name)
	command := filepath.Join(p.commands, name+".md")
	link := filepath.Join(p.claude, name)
	if exists(body) {
		return fmt.Errorf("skill path already exists: %s", body)
	}
	if exists(command) {
		return fmt.Errorf("command name already exists: %s", command)
	}
	if exists(link) {
		return fmt.Errorf("claude path already exists: %s", link)
	}
	if err := run(c.Root, stdout, stderr, "npx", "skills@1", "add", source, "--skill", name, "--agent", "universal", "-y"); err != nil {
		return err
	}
	if fi, err := os.Stat(filepath.Join(body, "SKILL.md")); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("skills.sh did not install %s", name)
	}
	lf, err = readLock(p.lock)
	if err != nil {
		return err
	}
	if !lf.skills.has(name) {
		return fmt.Errorf("skills.sh did not lock %s", name)
	}
	if err := os.MkdirAll(p.claude, 0o755); err != nil {
		return err
	}
	if err := os.Symlink("../../.agents/skills/"+name, link); err != nil {
		return err
	}
	return reviewPaths(p, stdout)
}

func update(c checkout.Checkout, args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return errors.New("update expects 0 argument(s)")
	}
	if err := run(c.Root, stdout, stderr, "npx", "skills@1", "update", "--project", "--yes"); err != nil {
		return err
	}
	return reviewPaths(pathsFor(c), stdout)
}

func remove(c checkout.Checkout, args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return errors.New("remove expects 1 argument(s)")
	}
	name := args[0]
	if err := validateName(name); err != nil {
		return err
	}
	p := pathsFor(c)
	lf, err := readLock(p.lock)
	if err != nil {
		return err
	}
	if !lf.skills.has(name) {
		return fmt.Errorf("%s is not in skills-lock.json", name)
	}
	body := filepath.Join(p.skills, name)
	link := filepath.Join(p.claude, name)
	if exists(body) {
		fi, err := os.Lstat(body)
		if err != nil {
			return err
		}
		if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to remove non-directory skill path: %s", body)
		}
	}
	if exists(link) {
		fi, err := os.Lstat(link)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refusing to remove non-symlink Claude path: %s", link)
		}
		target, err := os.Readlink(link)
		if err != nil {
			return err
		}
		if target != "../../.agents/skills/"+name {
			return fmt.Errorf("refusing to remove unexpected Claude link: %s", link)
		}
	}
	lf.skills.del(name)
	if err := writeLock(p.lock, lf); err != nil {
		return err
	}
	if err := os.RemoveAll(body); err != nil {
		return err
	}
	if err := os.Remove(link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return reviewPaths(p, stdout)
}

func check(c checkout.Checkout, args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return errors.New("check expects 0 argument(s)")
	}
	aliases, err := filepath.Rel(c.Root, c.Aliases())
	if err != nil {
		return err
	}
	if _, err := os.Stat(c.Aliases()); err != nil {
		return fmt.Errorf("shell entry point missing: %s", aliases)
	}
	cmd := exec.Command("bash", "-n", c.Aliases())
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bash -n %s: %w", aliases, err)
	}
	for _, cached := range []bool{false, true} {
		args := []string{"-C", c.Root, "diff"}
		if cached {
			args = append(args, "--cached")
		}
		args = append(args, "--check")
		cmd := exec.Command("git", args...)
		cmd.Stdout, cmd.Stderr = stdout, stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git %s: %w", strings.Join(args[2:], " "), err)
		}
	}
	return nil
}

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
		got, err := folderHash(filepath.Join(p.skills, name))
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

func verify(c checkout.Checkout, args []string, stdout, stderr io.Writer) error {
	if len(args) != 0 {
		return errors.New("verify expects 0 argument(s)")
	}
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
		fmt.Fprintf(stderr, "[warn] %d skill(s) unverifiable\n", unverifiable)
	}
	if drifted > 0 {
		return fmt.Errorf("%d skill(s) drifted", drifted)
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

// folderHash reimplements skills.sh's computeSkillFolderHash: sha256 over each regular file's forward-slash path then its bytes, files ordered by localeCompare, .git and node_modules skipped.
func folderHash(dir string) (string, error) {
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
		return "", err
	}
	c := collate.New(language.Und)
	sort.SliceStable(rels, func(i, j int) bool { return c.CompareString(rels[i], rels[j]) < 0 })
	h := sha256.New()
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		h.Write([]byte(rel))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func reviewPaths(p paths, stdout io.Writer) error {
	if insideWorkTree(p.root) {
		fmt.Fprint(stdout, "\nLocked-skill changes:\n")
		within := func(abs string) string {
			rel, err := filepath.Rel(p.root, abs)
			if err != nil {
				return abs
			}
			return rel
		}
		cmd := exec.Command("git", "-C", p.root, "status", "--short", "--", within(p.skills), within(p.lock), within(p.claude))
		cmd.Stdout = stdout
		cmd.Stderr = io.Discard
		_ = cmd.Run()
	}
	fmt.Fprint(stdout, "\nReview the upstream body and run: harness locks check\n")
	return nil
}

func insideWorkTree(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}
