package changemap

import (
	"fmt"
	"path"
	"strings"
)

// Match reports whether the slash-separated name matches pattern. Each segment follows path.Match, so *, ?, and [...] never cross a slash, and a segment that is exactly ** matches zero or more whole segments. The standard library has no recursive wildcard, and a critical path such as .github/** needs one.
func Match(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			rest := pattern[1:]
			for skip := 0; skip <= len(name); skip++ {
				if matchSegments(rest, name[skip:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], name[0]); !ok {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}

// ValidateGlob rejects a pattern that could never be a repo-relative glob: an empty or absolute one, one with a .. or empty segment, or one path.Match calls malformed. An empty segment (a//b, or a trailing slash) is refused rather than accepted because it can never match a changed file and would fail silently.
func ValidateGlob(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("empty pattern")
	}
	if strings.HasPrefix(pattern, "/") {
		return fmt.Errorf("absolute path; use a repo-relative pattern")
	}
	for _, segment := range strings.Split(pattern, "/") {
		switch {
		case segment == "":
			return fmt.Errorf("empty segment; drop the doubled or trailing slash")
		case segment == "..":
			return fmt.Errorf(".. segment; patterns stay inside the repository")
		case segment == "**":
			continue
		}
		// path.Match reports a malformed pattern even when the text cannot match.
		if _, err := path.Match(segment, "x"); err != nil {
			return fmt.Errorf("malformed pattern %q: %v", segment, err)
		}
	}
	return nil
}
