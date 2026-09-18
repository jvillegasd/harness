// Package harness carries the product assets inside the binary, so an install needs no checkout.
package harness

import "embed"

// Assets are the files an install unpacks and links: the policy, the aliases, the commands, the skills and their lock, the roles, and the client settings, at their checkout-relative paths. The lock tells an install which skills are locally authored and must carry the invocation guards.
//
//go:embed POLICY.md aliases.sh commands skills skills-lock.json roles clients
var Assets embed.FS
