package reviewledger

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
)

var pullKeyRe = regexp.MustCompile(`^pr-([0-9]+)$`)

// PruneMarks reads, for each pr-<n> directory prune lists, its pull request's state, and marks the ones still open, since `land check` reads that ledger. A failed read marks the directory's forge state unavailable. Other directories get no mark.
func PruneMarks(dirs []string, forge Forge) map[string]string {
	marks := map[string]string{}
	for _, d := range dirs {
		m := pullKeyRe.FindStringSubmatch(filepath.Base(d))
		if m == nil {
			continue
		}
		out, err := forge("pr", "view", m[1], "--json", "state")
		var pull struct{ State string }
		if err == nil {
			err = json.Unmarshal(out, &pull)
		}
		switch {
		case err != nil:
			marks[d] = "forge state unavailable"
		case pull.State == "OPEN":
			marks[d] = fmt.Sprintf("pull request #%s is open", m[1])
		}
	}
	return marks
}
