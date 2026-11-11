// Package harness carries the product assets inside the binary, so an install needs no checkout.
package harness

import "embed"

// Assets are the files an install unpacks and links: the floor, the aliases, the skills and their lock, the roles, and the client settings, at their checkout-relative paths. The lock tells an install which skills are locally authored and so checked for invocation guards.
//
//go:embed FLOOR.md aliases.sh skills skills-lock.json roles clients
var Assets embed.FS
