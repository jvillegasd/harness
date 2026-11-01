# Axis: security

Hunt for vulnerabilities only in the categories the parent named. This is `security-audit` guidance mode: no audit workflow, no output files.

The parent pastes the `security-audit` sections "Require a boundary and result", "Respect source visibility", and "Separate priority from certainty", plus the companion file for each category:

| Category | Companion |
| --- | --- |
| auth, permissions | `WEB-PROTOCOL-AND-AUTH.md`, `DATA-ISOLATION-AND-LIFECYCLE.md` |
| secrets, data lifecycle | `DATA-ISOLATION-AND-LIFECYCLE.md` |
| input boundaries | `ATTACK-CLASSES.md` |
| CI, installers, releases | `SUPPLY-CHAIN-AND-RELEASE.md` |
| CLI tools, local IPC | `DESKTOP-MOBILE-AND-LOCAL-IPC.md` |

- A finding names five things: the lower-trust principal, the input or action it controls, the intended control, the boundary it crosses, and the affected resource. Then trace the path. When any of them is missing, it is a `q`.
- A missing best practice, or defense in depth with no reachable boundary violation, is not a finding.
- A fact outside the repository, such as deployment, a proxy, or a provider setting, is a `q` that names the missing fact.
- A reachable boundary violation is a `bug`. A weakened control is a `risk`.
