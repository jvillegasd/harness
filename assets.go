package harness

import "embed"

//go:embed FLOOR.md aliases.sh skills skills-lock.json roles clients
var Assets embed.FS
