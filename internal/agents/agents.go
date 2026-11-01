// Package agents renders every assigned role into one agent definition per client, replacing the Python renderer the installer used to exec.
//
// The assignments file names every role's definition and each client's default model and effort, where `inherit` means the session's own choice. The machine's `harness models` choice arrives as Assigned values, which override their own client's fields. Each entry under `roles` names its canonical definition, relative to the assignments file; a per-client override there is accepted only without Assigned values, which is how the renderer's own tests exercise validation, and the installer's machine path refuses it. A field resolves from the override, else the client default, and generation prints every resolution with its source. A role with no definition is an assignment waiting for its role: it is validated and reported, and nothing is generated for it.
//
// Generation writes <output-dir>/<client>/<role>.md for every defined role and client, removes any other file under <output-dir>, and prints the resolved assignments, whether the output changed, the enforcement gaps, and the unverified capabilities. It exits 0 whether or not it had to change anything, and reports whether the output was already current. An invalid or unsupported assignment, an incomplete role, a role asking for other access than its own, or a missing required role exits 1 before anything is written; nothing is substituted.
//
// Output paths and their ancestors must not be symlinks; both generation and checking refuse them before reconciliation.
//
// Checking writes nothing. It reports each output that is missing, drifted, unmanaged, or no longer carries its access level's restrictions, and exits 1 when it found any.
package agents

import "errors"

// review launches the reviewer, triage and research launch code-search, and POLICY.md names the executor, so each must exist with exactly this access. Every other role is read-search: only the executor may implement.
var required = map[string]string{
	"executor":    "implement",
	"code-search": "read-search",
	"reviewer":    "read-search",
}

// Documented agent frontmatter values: https://code.claude.com/docs/en/sub-agents (model, effort), https://opencode.ai/docs/agents/ (model, plus provider options such as OpenAI's reasoningEffort), and Codex's custom agent files (model, model_reasoning_effort). OpenCode passes effort to the provider, so it is accepted only beside an openai/ model.
var supported = map[string][]field{
	"claude":   {{"model", `inherit|sonnet|opus|haiku|fable|claude-[a-z0-9.-]+`}, {"effort", `inherit|low|medium|high|xhigh|max`}},
	"codex":    {{"model", `inherit|[a-z0-9][a-z0-9.-]*`}, {"effort", `inherit|low|medium|high|xhigh|max|ultra`}},
	"opencode": {{"model", `inherit|[a-z0-9._-]+/[A-Za-z0-9._:/-]+`}, {"effort", `inherit|none|minimal|low|medium|high|xhigh`}},
}

type field struct{ name, pattern string }

const access = "Read and search the repository to follow callers and context. Never edit a file, fetch from the web, delegate to another agent, or run project or general shell commands. The only command exception is Orca lifecycle messaging under the live worker preamble."

var claudeControls = map[string][]string{
	"read-search": {"tools: Read, Grep, Glob, Bash", "permissionMode: dontAsk", "hooks:", "  PreToolUse:", "    - matcher: Bash", "      hooks:", "        - type: command", `          command: 'test -n "$HARNESS_LIFECYCLE_GUARD" || exit 2; "$HARNESS_LIFECYCLE_GUARD" models lifecycle-guard || exit 2'`},
	"implement":   {"disallowedTools: Agent"},
}

// OpenCode appends an agent's rules after its defaults and the last matching rule wins. A read-search role denies every permission and re-allows only reading, keeping the default .env guard a bare `read: allow` would override; whether MCP tools resolve through the same wildcard is unverified. The executor keeps the defaults and denies delegation plus the commit, push, worktree, and forge commands its parent owns.
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
		"    \"orca worktree rm*\": deny\n" +
		"    \"orca-ide worktree rm*\": deny\n" +
		"    \"orca-dev worktree rm*\": deny\n" +
		"    \"wt remove*\": deny\n" +
		"    \"wt merge*\": deny\n" +
		"    \"wt step commit*\": deny\n" +
		"    \"gh *\": deny\n",
}

// A Codex agent file cannot set its sandbox or approval policy, which it inherits from the parent, so the only control it carries is turning features off: a read-search role loses connectors and plugins, and the executor keeps the defaults.
var codexControls = map[string]string{
	"read-search": "[features]\napps = false\nplugins = false\n",
	"implement":   "",
}

// The developer_instructions literal string opens and closes on its own lines, so the body never touches a delimiter and a check can find where the controls start.
const (
	codexInstructions = "developer_instructions = '''\n"
	codexClose        = "\n'''\n"
)

// codexTail is what follows the instructions: a blank line and the level's controls, or nothing.
func codexTail(level string) string {
	if codexControls[level] == "" {
		return ""
	}
	return "\n" + codexControls[level]
}

// Known from each client's documentation. What no control covers is a gap; what a control should cover but only an installed client can prove is unverified.
var gaps = []string{
	"gap: claude: the executor keeps Bash, because a disallowedTools specifier removes the whole tool, so its commit, push, pull request, merge, and tracker limits are instructions only",
	"gap: claude: a model parameter passed at launch overrides an agent's model frontmatter, and an availableModels allowlist substitutes a blocked model with a warning, so the policy forbids the parameter and neither is enforced",
	"gap: codex: an agent file inherits the parent's sandbox and approval policy, so a native Codex subagent's read-search and executor limits are instructions only",
	"gap: codex: an Orca launch runs in the workspace-write sandbox with network on, because the sandbox otherwise blocks the Orca CLI, so commit is enforced by the read-only .git while gh, push of existing commits, and a read-search role's writes are instructions only",
	"gap: opencode: the executor's bash denies match command prefixes, so the same action spelled another way is not denied, including an Orca executable taken from ORCA_CLI_COMMAND, and the denied orca worktree rm spelling came from the installed Orca CLI's help rather than this tree, so a new Orca needs it re-verified",
}

var unverified = []string{
	"unverified: every restriction above is static configuration until an installed client is probed at runtime",
	"unverified: opencode applying the * deny to MCP tools",
	"unverified: codex applying a custom agent's features block to a spawned subagent",
	"unverified: the ccyolo, cxyolo, and ocyolo launchers leaving agent restrictions in force, since each bypasses or overrides the parent's permissions",
	"unverified: diff delivery's escalation when a launch prompt cannot carry the complete snapshot, which is protocol rather than exercised behavior",
	"unverified: claude applying a main-thread --agent session's model and effort frontmatter, and the session's effort when effort is absent",
	"unverified: opencode sending an agent's reasoningEffort with its model requests",
	"unverified: claude disallowedTools: Agent and opencode task: deny stopping the executor from delegating",
}

// fail reports an invalid or unsupported assignment, which Generate and Check surface as exit 1.
func fail(msg string) error { return errors.New(msg) }
