# Axis: standards

Judge the diff against the documented standards you were given, then invoke the `ponytail` skill for simplicity checks and apply the smell baseline below.

- A documented standard always wins. Where it endorses something the baseline would flag, drop the smell.
- Skip anything a formatter, linter, type checker, or test in the repository already enforces.
- A breach of a documented hard rule cites the file and the rule. It is a `risk`, or a `bug` when it breaks behavior.
- `ponytail` and the smell baseline are judgement calls, reported as `nit` with the label "possible <smell>".
- Report at most five nits: the ones that cost the next reader most.

## Smell baseline

Each entry describes a smell and its possible fix.

- **Mysterious Name**: a name that does not reveal what the thing does or holds. Rename it; if no honest name comes, the design is murky.
- **Duplicated Code**: the same logic shape in more than one hunk or file. Extract the shared shape.
- **Feature Envy**: a function that reaches into another object's data more than its own. Move it onto that data.
- **Data Clumps**: the same few fields or parameters travelling together. Bundle them into one type.
- **Primitive Obsession**: a primitive or string standing in for a domain concept. Give the concept its own small type.
- **Repeated Switches**: the same switch or if-cascade on the same type in several places. Use one map or polymorphism.
- **Shotgun Surgery**: one logical change scattered across many files. Gather what changes together.
- **Divergent Change**: one file edited for several unrelated reasons. Split it so each part changes for one reason.
- **Speculative Generality**: hooks or parameters for needs the requirements do not have. Inline them back.
- **Message Chains**: long `a.b().c().d()` navigation. Hide the walk behind one call.
- **Middle Man**: a function or type that mostly delegates onward. Call the real target directly.
- **Refused Bequest**: an implementer that ignores most of what it inherits. Use composition.
