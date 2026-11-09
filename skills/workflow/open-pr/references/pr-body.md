# Pull request body

Use the repository's own template (`.github/pull_request_template.md` or `.github/PULL_REQUEST_TEMPLATE/`) when it has one, and fit the sections below into it. Otherwise use this one. A reviewer should be able to read it in under a minute and know which files need their attention.

Rules:

- Every changed file appears in the review guide exactly once, in the tier `change-map` settled on after raising. Files of the same tier that need the same check may share a row. A locked skill body that still matches its lock is the one grouped `mechanical` row `change-map` gives it, with its file count, rather than a row per file.
- A tier raised above the `harness change-map` floor says so: `behavioral (raised from structural: changes help output)`.
- Rows go from the highest tier to the lowest. `critical` and `behavioral` rows name the one fact the file is safe because of and how far it is proven (said, cited, walked, ran, reproduced). A fact at "said" or "cited" is written as unproven.
- Leave out a section only where the template says it may be omitted. Write "None" rather than deleting a required section.

```markdown
## Why

<The problem in one to three sentences, from the reader's side. Link the issue or ticket.>

## What changed

<The smallest view that makes the change clear: a few bullets, pseudocode, a call tree, or a small Mermaid diagram.>

## Review guide

Tiers: critical N, behavioral N, structural N, tests-docs N, mechanical N

| Tier | File | What to check | Safe because |
| --- | --- | --- | --- |
| critical | `path/to/file` L40-52 | <what a mistake here would break> | <fact> (<ladder step>) |
| behavioral | `path/to/file` | <the behavior that changes> | <fact> (<ladder step>) |
| structural | `path/a`, `path/b` | Skim: claims to preserve behavior | Existing tests unchanged and passing |
| tests-docs | `docs/x.md`, `test/y_test.go` | Skim: assertions that cannot fail, docs that disagree with the code | |
| mechanical | `go.sum` | Skip | <the deterministic check that classified it> |

Merge danger: <one-way or two-way> door. Blast radius: <a few words>.

## Decisions you did not ask for

- <decision>: <reason>

## Evidence

- `<command>` at `<short sha>`: <result>
- Not run: <check>, because <reason>

## Risks

<Material unresolved risks other than review findings, or "None".>

<Closes #N, only for scope actually completed. Omit otherwise.>

<Reviewed: <base>..<sha>, or Review: not run (level <level>). When any review slice stayed unreviewed, write neither line and name the gap under Risks.>
```
