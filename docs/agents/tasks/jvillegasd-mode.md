# jvillegasd-mode: fewer planning pieces and hidden helpers

Branch: `refactor/jvillegasd-mode`. Status: shipped for review.

Request (live session, 2026-11-16): "I want to have fewer pieces in planning too, grilling with docs, we can merge spec and tickets into one called epic that follows jira princples of a big ticket (spec) with the verticals (tickets). For build, we already have the autopilot and the implement skill, we may need merge them too. Also I want to rename work skill to jvillegasd-mode and hide whats isn't an entry point. regarding evals, I want to relax this, keep manual evals like sjdonado. regarding the playbooks, lets see which skills and principles overlaps, also which principles we can trim"

Follow-up answers: entry points are "mode and the skills that are not triggered by others"; build is "One `build`, sized by input"; evals "better skip for now"; floor duplicates: "remove the floor prose that makes these principles duplicates"; simplicity duplicates: explain first; design duplicates: "address as proposed"; support duplicates: dedupe handoff and continuity, keep the others.

## Requirements

- `epic` replaces `spec` and `tickets`: write the big ticket, agree its seams, cut it into vertical slices. `grill-with-docs` stays a locked entry point ("grill-with-docs needs to be locked, epic cover spec and tickets").
- `build` absorbs `implement`: one slice inline with `tdd`, several slices fanned out.
- `work` is renamed `jvillegasd-mode`, and `continuity` is renamed `checkpoint` ("also rename continuity to checkpoint"). `retro` and `teach` are removed ("remove retro and teach skills").
- Only `workflow/` skills are discoverable: the mode and the skills no other skill runs. Every other group installs to `~/.harness/skills/<name>`, where the floor tells the agent to read a named skill from.
- Delete `principle-outcome-oriented-execution`, `principle-exhaust-the-design-space`, `principle-redesign-from-first-principles`, and `handoff`.
- Remove the floor lines that duplicate `principle-never-block-on-the-human` and `principle-guard-the-context-window`; delete `ponytail` and keep `principle-subtract-before-you-add`.
- Evals unchanged.

## Phases

1. Rename `work` to `jvillegasd-mode`.
2. `epic` from `spec`, `tickets`, `grill-with-docs`.
3. `build` absorbs `implement`.
4. Regroup so `workflow/` is the entry-point set, and install the other groups hidden.
5. Trim the agreed principles and `handoff`.
6. Checks, install into a throwaway home, Finish.

## Decisions

| Decision | Instead of | By | Why | Evidence |
| --- | --- | --- | --- | --- |
| Visibility follows the group: `workflow/` is discoverable, every other group is hidden | A list of visible skill names | agent | The group rule already says `workflow/` holds only what the human invokes, so no second list can drift from it | `AGENTS.md` Directories row for `skills/<group>/<name>/` |
| Hidden skills install once to `~/.harness/skills/<name>` for every client | Per-client hidden roots | agent | One path the floor can name for every client | `internal/install/state.go` `desired` |
| Keep `benchmark-checklist` and `principle-minimize-reader-load` | Deleting them as duplicates | agent | Re-reading showed each owns a procedure the other lacks | Explanation given in the session |
| `build` keeps the name and takes the single-slice path | Naming it `implement` | human | Answer to the build-merge question | Session |
| The verification, formal-verification, `statechart`, and `e2e-verification` skills stay entry points | Hiding them in `assurance/` | agent | Other skills only suggest them to the human, so nothing else runs them | Review finding; `e2e-verification` line 14, `change-map` line 35 |
| Floor drops its ask and inline-delegation lines; the mode's principles own them | Keeping them in the floor | human | "yes thats ok because I intended to use jvillegas mode as default unless grilling or the other skills called" | Session |
| Delete `ponytail`, keep `principle-subtract-before-you-add` | Deleting the principle | human | "delete ponitail, keep principle" | Session |
| `build` picks its path by the epic's total slices | By the slices ready at first | agent | A chained epic has one ready slice and would stop after it | Review finding; `build` line 10 |

## State and next action

Playbook: large, step 5. Shipped for review. Evidence: `go vet`, `gofmt -l`, `go test ./...`, `go build`, `git diff --check` pass at 1842d66; a throwaway-home `harness install skills` installed only `workflow/` into the client skill folders at b5ff2dc. Open: the locked `principle-attack-the-premise` links to the deleted redesign principle.
