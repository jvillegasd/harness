# Harness AI shell aliases. Sourced from ~/.zshrc by `harness install`.
#
# Yolo mode: no approval prompts, for a client running inside a worktree or a
# container you are willing to lose. The floor in FLOOR.md and its stop rule
# are what keep this honest; the client's prompt is not.

alias ccyolo='claude --dangerously-skip-permissions'

# Codex's bypass drops both the approval prompts and its sandbox.
alias cxyolo='codex --dangerously-bypass-approvals-and-sandbox'

# OpenCode has no bypass flag. Permissions are configuration, and
# OPENCODE_PERMISSION overrides them for one run.
alias ocyolo='OPENCODE_PERMISSION='"'"'{"edit":"allow","bash":"allow","webfetch":"allow"}'"'"' opencode'
