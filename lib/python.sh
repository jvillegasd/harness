#!/usr/bin/env bash
# The one declaration of which Python interpreters this harness consumes.
#
# Sourced, never executed, and always after its caller has defined `log` and `die`: `harness install` defines both before sourcing it. The dotfiles platform scripts install the same set from their own copy of this file; here the harness only asserts them, because the harness is what consumes the interpreters and so is where a missing one has to fail loudly.

# Newest first. Homebrew supplies none of these: uv resolves a managed version
# through its own registry rather than through PATH, so `uv run --python 3.14`
# finds them whatever pyenv or Homebrew have done to `python3`. 3.11 was here
# only for the SEO runtime's pin and went with it.
PYTHON_VERSIONS=(3.14)

# Asked of uv rather than of PATH, because that is how every caller reaches
# them: a shadowing Homebrew or pyenv python3.<minor> would pass a PATH check
# while uv still had nothing to resolve.
assert_pythons() {
    local version
    command -v uv >/dev/null || die "Missing required command: uv"
    for version in "${PYTHON_VERSIONS[@]}"; do
        # uv's own "no interpreter found" goes to stderr; the actionable
        # message is the one below, and two errors for one problem is noise.
        uv python find --managed-python "$version" >/dev/null 2>&1 ||
            die "Missing uv-managed Python $version; install it with 'uv python install $version'."
    done
}
