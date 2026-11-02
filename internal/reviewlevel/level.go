// Package reviewlevel keeps the human-set review level behind `harness review level`. Review findings and their dispositions live on the pull request, in the verdict comment `review` posts.
package reviewlevel

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/jvillegasd/harness/internal/statefile"
)

// Levels are the review levels a human may choose, cheapest first. The level caps the reviews an agent starts on its own; a human's explicit request for a review runs whatever the level says.
var Levels = []string{"off", "medium", "high"}

// DefaultLevel applies when no level is stored.
const DefaultLevel = "medium"

const levelUsage = `Usage: harness review level [--json]
       harness review level off|medium|high

Print the review level, or set it. The level caps the reviews an agent starts
on its own; an explicit request for a review always runs.

  off     Agents start no review.
  medium  Agents review every pull request above tests-docs, and local work
          only when critical or a named concrete risk, with one hunter and
          one batched refuter. The default.
  high    Agents review structural and riskier changes at the depth the
          change-map tier sets, with batched verification.

At medium and high, /land holds a pull request above tests-docs until a
review covers its current patch; at off it reports the gap.

The level is stored in ~/.harness/state.json. Only a human sets it.
`

type reviewBlock struct {
	Level string `json:"level"`
}

// LoadLevel returns the stored level and its source, "global" or "default".
func LoadLevel(path string) (level, source string, err error) {
	doc, err := statefile.Read(path)
	if err != nil {
		return "", "", err
	}
	raw, ok := doc["review"]
	if !ok {
		return DefaultLevel, "default", nil
	}
	var b reviewBlock
	if err := json.Unmarshal(raw, &b); err != nil || !slices.Contains(Levels, b.Level) {
		return "", "", fmt.Errorf("%s: unreadable review level %s; run `harness review level %s`", path, raw, DefaultLevel)
	}
	return b.Level, "global", nil
}

// SaveLevel stores level and keeps every other key.
func SaveLevel(path, level string) error {
	if !slices.Contains(Levels, level) {
		return fmt.Errorf("unknown level %q; choose %s", level, strings.Join(Levels, ", "))
	}
	doc, err := statefile.Read(path)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(reviewBlock{level})
	if err != nil {
		return err
	}
	doc["review"] = raw
	return statefile.Write(path, doc)
}

const usage = `Usage: harness review level [--json]
       harness review level off|medium|high

Commands:
  level  Print or set the review level: off, medium (default), or high
`

// Command runs `harness review` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "level":
		return level(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness review: unknown command %q; run harness review --help\n", args[0])
	return 2
}

func level(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var value []string
	for _, a := range args {
		switch a {
		case "-h", "--help", "help":
			fmt.Fprint(stdout, levelUsage)
			return 0
		case "--json":
			asJSON = true
		default:
			value = append(value, a)
		}
	}
	// Setting takes the bare value alone, so the setting forms are exactly the prefixes a client can deny.
	if len(value) > 1 || (len(value) == 1 && (asJSON || !slices.Contains(Levels, value[0]))) {
		fmt.Fprintf(stderr, "harness review level: expected one of %s\n%s", strings.Join(Levels, ", "), levelUsage)
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "harness review level: %v\n", err)
		return 1
	}
	path := statefile.Path(home)
	if len(value) == 1 {
		if err := SaveLevel(path, value[0]); err != nil {
			fmt.Fprintf(stderr, "harness review level: %v\n", err)
			return 1
		}
	}
	lvl, source, err := LoadLevel(path)
	if err != nil {
		fmt.Fprintf(stderr, "harness review level: %v\n", err)
		return 1
	}
	if asJSON {
		out, _ := json.Marshal(map[string]string{"level": lvl, "source": source})
		fmt.Fprintln(stdout, string(out))
		return 0
	}
	fmt.Fprintf(stdout, "%s (%s)\n", lvl, source)
	return 0
}
