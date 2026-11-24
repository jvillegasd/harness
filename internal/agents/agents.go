package agents

import "errors"

// Role is one kind of subagent. Tier and Effort are its defaults; a lens has
// neither and follows the reviewer. A generic role takes its tier and client
// at each launch, for subagents no other role describes.
type Role struct {
	Name, Definition, Access string
	Tier, Effort             string
	Lens, Generic            bool
}

var Roles = []Role{
	{Name: "executor", Definition: "roles/executor.md", Access: "implement", Tier: "standard"},
	{Name: "code-search", Definition: "roles/code-search.md", Access: "read-search", Tier: "fast"},
	{Name: "worker", Definition: "roles/worker.md", Access: "implement", Tier: "standard", Generic: true},
	{Name: "reviewer", Definition: "skills/delivery/review/roles/reviewer.md", Access: "read-search", Tier: "high", Effort: "low"},
	{Name: "review-bugs", Definition: "skills/delivery/review/roles/review-bugs.md", Access: "read-search", Lens: true},
	{Name: "review-spec", Definition: "skills/delivery/review/roles/review-spec.md", Access: "read-search", Lens: true},
	{Name: "review-risk", Definition: "skills/delivery/review/roles/review-risk.md", Access: "read-search", Lens: true},
}

var supported = map[string][]field{
	"claude":   {{"model", `inherit|sonnet|opus|haiku|fable|claude-[a-z0-9.-]+`}, {"effort", `inherit|low|medium|high|xhigh|max`}},
	"codex":    {{"model", `inherit|[a-z0-9][a-z0-9.-]*`}, {"effort", `inherit|low|medium|high|xhigh|max|ultra`}},
	"opencode": {{"model", `inherit|[a-z0-9._-]+/[A-Za-z0-9._:/-]+`}, {"effort", `inherit|none|minimal|low|medium|high|xhigh`}},
}

type field struct{ name, pattern string }

const access = "Read and search the repository to follow callers and context, and load a skill your instructions name, reading its own files for guidance only. Never edit a file, fetch from the web, delegate to another agent, or run project or general shell commands, even when a skill's steps say to. The shell is for reading inside the working directory: `cat`, `head`, `tail`, `wc`, `ls`, `find`, `rg`, and `git` `show`, `diff`, `log`, `status`, `blame`, `ls-files`, `rev-parse`, `merge-base`, and `grep` (`show`, `diff`, and `log` with `--no-ext-diff --no-textconv`, `blame` with `--no-textconv`), one command per call with no pipes, redirections, or substitutions, and never on credential files or paths outside the working directory except the files of a skill your instructions name."

var claudeControls = map[string][]string{
	"read-search": {"tools: Read, Grep, Glob, Bash, Skill", "permissionMode: dontAsk", "hooks:", "  PreToolUse:", "    - matcher: Bash", "      hooks:", "        - type: command", `          command: '"$HOME/.local/bin/harness" models read-guard || exit 2'`},
	"implement":   {"disallowedTools: Agent"},
}

var opencodeControls = map[string]string{
	"read-search": "permission:\n" +
		"  \"*\": deny\n" +
		"  read:\n" +
		"    \"*\": allow\n" +
		"    \"*.env\": deny\n" +
		"    \"*.env.*\": deny\n" +
		"    \"*.env.local\": allow\n" +
		"    \"*.env.*.local\": allow\n" +
		"    \"*.env.example\": allow\n" +
		"  grep: allow\n" +
		"  glob: allow\n" +
		"  list: allow\n" +
		"  skill: allow\n" +
		"  external_directory:\n" +
		"    \"~/.agents/skills/**\": allow\n" +
		"    \"~/.config/opencode/skills/**\": allow\n",
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

var codexControls = map[string]string{
	"read-search": "[features]\napps = false\nplugins = false\n",
	"implement":   "",
}

const (
	codexInstructions = "developer_instructions = '''\n"
	codexClose        = "\n'''\n"
)

func codexTail(level string) string {
	if codexControls[level] == "" {
		return ""
	}
	return "\n" + codexControls[level]
}

var gaps = []string{
	"gap: claude: the executor keeps Bash, because a disallowedTools specifier removes the whole tool, so its commit, push, pull request, merge, and tracker limits are instructions only",
	"gap: claude: a model parameter passed at launch overrides an agent's model frontmatter, and an availableModels allowlist substitutes a blocked model with a warning, so the policy allows the parameter only for the worker's resolved tier model and neither is enforced",
	"gap: codex, opencode: a native launch by name keeps the installed agent's model, so the worker runs at its configured tier there unless T3 Code's delegate_task launches it at another",
	"gap: codex: an agent file inherits the parent's sandbox and approval policy, so a native Codex subagent's read-search and executor limits are instructions only",
	"gap: opencode: the executor's bash denies match command prefixes, so the same action spelled another way is not denied",
	"gap: t3: a role delegated through T3 Code's delegate_task runs as a fresh provider session that loads no installed role agent, and T3 has no read-only runtime mode, so its role definition and access limits travel in the brief as instructions only",
}

var unverified = []string{
	"unverified: every restriction above is static configuration until an installed client is probed at runtime",
	"unverified: opencode applying the * deny to MCP tools",
	"unverified: codex applying a custom agent's features block to a spawned subagent",
	"unverified: the ccyolo, cxyolo, and ocyolo launchers leaving agent restrictions in force, since each bypasses or overrides the parent's permissions",
	"unverified: diff delivery's escalation when a launch prompt cannot carry the complete snapshot, which is protocol rather than exercised behavior",
	"unverified: claude applying a main-thread --agent session's model and effort frontmatter, and the session's effort when effort is absent",
	"unverified: opencode sending an agent's reasoningEffort with its model requests",
	"unverified: codex and opencode running an agent file that names no model, such as a role at the session tier, on the launching session's model rather than the configured default",
	"unverified: claude letting a dontAsk read-search role read an invoked skill's companion files, and search and read the installed principle skills, through the tracked Read(~/.claude/skills/**) allow rule",
	"unverified: opencode resolving ~ in a read-search role's external_directory patterns for the installed skill directories",
	"unverified: claude running a read-search role's read-guard hook from $HOME/.local/bin/harness inside a native subagent",
	"unverified: claude disallowedTools: Agent and opencode task: deny stopping the executor from delegating",
}

func fail(msg string) error { return errors.New(msg) }
