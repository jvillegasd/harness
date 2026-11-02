package models

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// readFlags lists, per allowed read command, the flags that would write, run another program, follow a link out of the tree, read outside it, or never exit. A short-flag cluster is refused when it holds any of the letters after "-".
var readFlags = map[string][]string{
	"cat":  nil,
	"head": nil,
	"ls":   nil,
	"wc":   {"--files0-from"},
	"tail": {"-f", "-F", "--follow", "--retry"},
	"find": {"-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprint0", "-fprintf", "-fls", "-L", "-follow"},
	"rg":   {"--pre", "--no-ignore", "--hidden", "--follow", "--search-zip", "-u", "-L", "-z", "-."},
	"git":  {"--output", "--ext-diff", "--textconv", "--no-index", "--open-files-in-pager", "-O"},
}

// gitReads are the git subcommands that only read the repository, each with the flags it must carry. Git runs a configured external diff driver or textconv filter that .gitattributes selects, so a subcommand that prints diffs or blame must turn both off.
var gitReads = map[string][]string{
	"show": {"--no-ext-diff", "--no-textconv"}, "diff": {"--no-ext-diff", "--no-textconv"}, "log": {"--no-ext-diff", "--no-textconv"},
	"blame": {"--no-textconv"}, "status": nil, "ls-files": nil, "rev-parse": nil, "merge-base": nil, "grep": nil,
}

// readCommand allows a read-only command whose every argument stays inside dir: no refused flag, no credential-shaped name, and no existing path that resolves outside dir. It runs nothing; a path that does not exist is left to the command to reject.
func readCommand(words []string, dir string) bool {
	refused, ok := readFlags[words[0]]
	if !ok || dir == "" {
		return false
	}
	args := words[1:]
	if words[0] == "git" {
		if len(args) == 0 {
			return false
		}
		required, ok := gitReads[args[0]]
		if !ok {
			return false
		}
		args = args[1:]
		for _, flag := range required {
			if !slices.Contains(args, flag) {
				return false
			}
		}
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	for _, arg := range args {
		if refusedFlag(arg, refused) {
			return false
		}
		for _, operand := range operands(arg) {
			if credentialShaped(operand) || !staysInside(operand, dir, root) {
				return false
			}
		}
	}
	return true
}

// operands lists every piece of an argument a command could read as a path: the argument itself, a long flag's value after "=", and every tail of a short-flag cluster, since a short flag may take its value attached (rg -f/etc/hosts).
func operands(arg string) []string {
	if !strings.HasPrefix(arg, "-") {
		return []string{arg}
	}
	if strings.HasPrefix(arg, "--") {
		if _, value, ok := strings.Cut(arg, "="); ok && value != "" {
			return []string{value}
		}
		return nil
	}
	var tails []string
	for i := 2; i < len(arg); i++ {
		tails = append(tails, arg[i:])
	}
	return tails
}

// staysInside reports whether an existing path resolves inside root; a path that does not exist is left to the command to reject.
func staysInside(path, dir, root string) bool {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	if _, err := os.Lstat(path); err != nil {
		return true
	}
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && (resolved == root || strings.HasPrefix(resolved, root+string(filepath.Separator))) && !credentialShaped(resolved)
}

func refusedFlag(arg string, refused []string) bool {
	for _, flag := range refused {
		if arg == flag || strings.HasPrefix(arg, flag+"=") || (strings.HasPrefix(flag, "--") && strings.HasPrefix(arg, flag+"-")) {
			return true
		}
		if len(flag) == 2 && flag[0] == '-' && len(arg) > 1 && arg[0] == '-' && arg[1] != '-' && strings.ContainsRune(arg[1:], rune(flag[1])) {
			return true
		}
	}
	return false
}

// credentialShaped mirrors the tracked Read denies in clients/claude.json, applied to every path-like piece of an argument, including a git revision's path after ":".
func credentialShaped(arg string) bool {
	for _, part := range strings.FieldsFunc(arg, func(r rune) bool { return r == '/' || r == ':' || r == '=' }) {
		switch {
		case strings.HasPrefix(part, ".env"), strings.HasPrefix(part, ".dev.vars"),
			strings.HasSuffix(part, ".pem"), strings.HasSuffix(part, ".key"),
			slices.Contains([]string{"secrets", "credentials", ".aws", ".ssh", ".npmrc", ".pypirc", "database.yml", "credentials.json"}, part):
			return true
		}
	}
	return false
}
