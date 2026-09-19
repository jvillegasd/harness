# Harness AI shell aliases. Sourced from ~/.zshrc by `harness install`.
#
# Yolo mode: no approval prompts, for a client running inside a worktree or a
# container you are willing to lose. The policy's escalation contract is what
# keeps this honest; the client's prompt is not.

alias ccyolo='claude --dangerously-skip-permissions'

# OpenCode has no bypass flag. Permissions are configuration, and
# OPENCODE_PERMISSION overrides them for one run.
alias ocyolo='OPENCODE_PERMISSION='"'"'{"edit":"allow","bash":"allow","webfetch":"allow"}'"'"' opencode'

# Lets an OpenCode session start a subagent with background=true, so it keeps working while reviewers run. Experimental in OpenCode 1.15.11 and later; an existing value wins.
export OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS="${OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS:-true}"
