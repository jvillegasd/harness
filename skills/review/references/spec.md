# Axis: spec

Judge whether the diff does what the requirements ask, and only that. The requirements were copied verbatim from their source. Quote the line each finding rests on.

- **Missing or partial**: a requirement the diff does not implement, or implements for only some of its cases.
- **Implemented wrong**: a requirement that looks implemented but behaves differently from what its line says.
- **Scope creep**: behavior nobody asked for. Name a broken public contract, an auth or data-boundary change, or a migration without a rollback explicitly.

A missing or wrong requirement is a `bug`. Scope creep is a `risk` when a user or caller can see it, and a `nit` otherwise. A requirement you cannot map to the diff either way is a `q`.

Keep the report under 400 words.
