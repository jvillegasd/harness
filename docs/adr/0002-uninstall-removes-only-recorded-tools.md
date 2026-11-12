# Uninstall removes only the tools the harness recorded installing, and only when asked

`harness install`, `harness update`, and `harness uninstall` select the same components by name, and `clients`, `packages`, and `mcps` carry tools as well as managed config. An install adds a tool only when it is missing from PATH and records how it added it; `harness uninstall` removes a tool only from that record, through the same method, and only when the tool's component is named or `--all` is given. A bare `harness uninstall` removes managed config alone, and no uninstall deletes a tool's data directory (`~/.engram`, `~/.agent-browser`, `~/.claude`, `~/.codex`).

## Considered options

- **Remove whatever is installed**: rejected, because node, gh, or a client the user ran long before the harness would go with it, and the harness cannot tell its own install from theirs without a record.
- **A bare uninstall removes tools too, mirroring a bare install**: rejected, because a removed client may be the one running the command, and a tool removal is the one step the harness cannot put back exactly as it was.

## Consequences

Machines installed before the record existed have no tool records, so every tool on them counts as present before the harness and stays. `~/.harness` outlives a config-only uninstall while it still records a tool, since deleting it would make that tool impossible to remove later; it goes, with the `harness models` and review-level choices, only when nothing recorded remains. The component that deploys the floor is renamed from `policy` to `floor`, and `mcp` to `mcps`, with existing records migrated on read, which supersedes ADR 0001's note that the `policy` key stays.
