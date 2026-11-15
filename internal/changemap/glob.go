package changemap

import (
	"fmt"
	"path"
	"strings"
)

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

		if _, err := path.Match(segment, "x"); err != nil {
			return fmt.Errorf("malformed pattern %q: %v", segment, err)
		}
	}
	return nil
}
