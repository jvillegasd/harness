# Installation and your files

These diagrams follow one managed copy, such as a skill, the floor, or a role definition. The installer compares its content hash with the record in `~/.harness/state.json`. They do not describe client settings or MCP registrations, which are reconciled separately. Tracked settings can overwrite in-app preferences.

## Installing

Run `harness install` to install the binary's embedded assets, or `go run ./cmd/harness install --skip-packages` to apply checkout assets without installing packages. Both change live client configuration.

```mermaid
stateDiagram-v2
    state "Inspect destination" as Inspect
    state "Absent" as Absent
    state "Matches desired content" as Matches
    state "Unchanged managed copy" as Managed
    state "Edited or conflicting copy" as Conflict
    state "Previous copy archived" as Archived
    state "Previous copy deleted" as Deleted
    state "Desired copy installed and recorded" as Installed
    state "Matching copy recorded" as Recorded
    state "Obsolete recorded copy" as Obsolete
    state "Removed" as Removed
    state "Preserved with warning" as Preserved

    [*] --> Inspect: Installer reads desired assets and previous records
    Inspect --> Absent: Desired path is missing
    Inspect --> Matches: Content already matches desired asset
    Inspect --> Managed: Content differs from desired but matches recorded hash
    Inspect --> Conflict: Content differs from desired and recorded hash or has no record
    Absent --> Installed: Installer writes desired copy
    Matches --> Recorded: Installer leaves content alone and records ownership
    Managed --> Installed: Installer replaces unchanged managed copy
    Conflict --> Archived: Default moves previous copy to backup
    Conflict --> Deleted: Destructive --no-backup override
    Archived --> Installed: Installer writes desired copy
    Deleted --> Installed: Installer writes desired copy
    Inspect --> Obsolete: Recorded path is no longer shipped
    Obsolete --> Removed: Existing content matches recorded hash
    Obsolete --> Preserved: Existing content differs from recorded hash
```

Archives go under `~/.dotfiles-backups/harness/`. `--no-backup` deletes conflicting content instead of archiving it. `--clear-backups` separately deletes those archives. Neither operation is recoverable through the harness.

An obsolete copy that is already absent needs no removal. Private entries outside the desired paths and recorded copies stay untouched. A managed copy can be a directory, so an edit anywhere inside it changes its hash. Managed-root symlinks are moved aside before installation.

## Uninstalling

Run `harness install --uninstall`. Removal asks for confirmation unless you pass `--yes`. Client and component filters narrow the selection.

```mermaid
stateDiagram-v2
    state "Removal requested" as Requested
    state "Nothing removed" as Cancelled
    state "Inspect recorded copy" as Inspect
    state "Kept for another client" as Shared
    state "Already absent" as Absent
    state "Removed" as Removed
    state "Edited copy preserved with warning" as Preserved

    [*] --> Requested: You request uninstall
    Requested --> Cancelled: You decline confirmation
    Requested --> Inspect: You confirm or supplied --yes
    Inspect --> Shared: Another installed client still needs this copy
    Inspect --> Absent: No remaining client needs it and path is missing
    Inspect --> Removed: No remaining client needs it and content matches recorded hash
    Inspect --> Preserved: No remaining client needs it and content differs from recorded hash
```

Uninstall removes ownership records for copies no remaining client needs, including edited copies it preserves. A preserved file is therefore no longer managed. Unrecorded private entries remain untouched.

Uninstall also removes selected managed settings, shell integration, and MCP registrations through their own rules. It leaves installed packages, backups, and user data. A full uninstall removes `~/.harness`, including saved model assignments and the review-level choice.
