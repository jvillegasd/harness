// Package harness carries the product assets inside the binary, so an install needs no checkout.
package harness

import "embed"

// Assets are the files an install unpacks and links: the policy, the aliases, the commands, the skills, the roles, and the client settings, at their checkout-relative paths.
//
//go:embed POLICY.md aliases.sh commands skills roles clients
var Assets embed.FS
