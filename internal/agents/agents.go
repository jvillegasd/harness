// Package agents renders every assigned role into one agent definition per client, replacing the Python renderer the installer used to exec.
//
// The assignments file names every role's definition and each client's default model and effort, where `inherit` means the session's own choice. The machine's `harness models` choice arrives as Assigned values, which override their own client's fields. Each entry under `roles` names its canonical definition, relative to the assignments file, and may override either field per client. A field resolves from the role's override, else the client default, and generation prints every resolution with its source. A role with no definition is an assignment waiting for its role: it is validated and reported, and nothing is generated for it.
//
// Generation writes <output-dir>/<client>/<role>.md for every defined role and client, removes any other file under <output-dir>, and prints the resolved assignments, whether the output changed, the enforcement gaps, and the unverified capabilities. It exits 0 whether or not it had to change anything, and reports whether the output was already current. An invalid or unsupported assignment, an incomplete role, a role asking for other access than its own, or a missing required role exits 1 before anything is written; nothing is substituted.
//
// Output paths and their ancestors must not be symlinks; both generation and checking refuse them before reconciliation.
//
// Checking writes nothing. It reports each output that is missing, drifted, unmanaged, or no longer carries its access level's restrictions, and exits 1 when it found any.
package agents

import "errors"

// adversarial-review launches the reviewers and the refuter and POLICY.md names the executor, so each must exist with exactly this access. Every other role is read-search: only the executor may implement.
var required = map[string]string{
	"executor":       "implement",
	"review-refuter": "read-search",
	"reviewer-a":     "read-search",
	"reviewer-b":     "read-search",
}

// Documented agent frontmatter values: https://code.claude.com/docs/en/sub-agents (model, effort) and https://opencode.ai/docs/agents/ (model, plus provider options such as OpenAI's reasoningEffort). OpenCode passes effort to the provider, so it is accepted only beside an openai/ model.
var supported = map[string][]field{
	"claude":   {{"model", `inherit|sonnet|opus|haiku|fable|claude-[a-z0-9.-]+`}, {"effort", `inherit|low|medium|high|xhigh|max`}},
	"opencode": {{"model", `inherit|[a-z0-9._-]+/[A-Za-z0-9._:/-]+`}, {"effort", `inherit|none|minimal|low|medium|high|xhigh`}},
}

type field struct{ name, pattern string }

const access = "Read and search only. Read and search the repository to follow callers and context. Never edit a file, run a command, fetch from the web, or delegate to another agent, even where the client would let you."

var claudeControls = map[string][]string{
	"read-search": {"tools: Read, Grep, Glob"},
	"implement":   {"disallowedTools: Agent"},
}

// OpenCode appends an agent's rules after its defaults and the last matching rule wins. A reviewer denies every permission and re-allows only reading, keeping the default .env guard a bare `read: allow` would override; whether MCP tools resolve through the same wildcard is unverified. The executor keeps the defaults and denies delegation plus the commit, push, worktree, and forge commands its parent owns.
var opencodeControls = map[string]string{
	"read-search": "permission:\n" +
		"  \"*\": deny\n" +
		"  read:\n" +
		"    \"*\": allow\n" +
		"    \"*.env\": deny\n" +
		"    \"*.env.*\": deny\n" +
		"    \"*.env.example\": allow\n" +
		"  grep: allow\n" +
		"  glob: allow\n" +
		"  list: allow\n",
	"implement": "permission:\n" +
		"  task: deny\n" +
		"  bash:\n" +
		"    \"git commit*\": deny\n" +
		"    \"git push*\": deny\n" +
		"    \"git worktree remove*\": deny\n" +
		"    \"wt remove*\": deny\n" +
		"    \"wt merge*\": deny\n" +
		"    \"wt step commit*\": deny\n" +
		"    \"gh *\": deny\n",
}

// Known from each client's documentation. What no control covers is a gap; what a control should cover but only an installed client can prove is unverified.
var gaps = []string{
	"gap: opencode: a diff file outside the project needs external_directory, which the agent's * deny refuses, so reviewers and the refuter there take the inline fallback",
	"gap: opencode: a standards file outside the project, such as the installed ponytail body, needs the same external_directory, so reviewers there get it inline after their loaded: line omits it",
	"gap: claude: the executor keeps Bash, because a disallowedTools specifier removes the whole tool, so its commit, push, pull request, merge, and tracker limits are instructions only",
	"gap: claude: a model parameter passed at launch overrides an agent's model frontmatter, and an availableModels allowlist substitutes a blocked model with a warning, so the policy forbids the parameter and neither is enforced",
	"gap: opencode: the executor's bash denies match command prefixes, so the same action spelled another way is not denied",
}

var unverified = []string{
	"unverified: every restriction above is static configuration until an installed client is probed at runtime",
	"unverified: claude reading a diff or standards file outside the project without a permission prompt",
	"unverified: opencode applying the * deny to MCP tools",
	"unverified: the ccyolo and ocyolo launchers leaving agent restrictions in force, since each bypasses or overrides the parent's permissions",
	"unverified: diff delivery's inline fallback and escalation, which are protocol rather than exercised behavior",
	"unverified: claude running a subagent on its model and effort frontmatter, and on the session's effort when effort is absent",
	"unverified: opencode sending an agent's reasoningEffort with its model requests",
	"unverified: claude disallowedTools: Agent and opencode task: deny stopping the executor from delegating",
}

// fail reports an invalid or unsupported assignment, which Generate and Check surface as exit 1.
func fail(msg string) error { return errors.New(msg) }
