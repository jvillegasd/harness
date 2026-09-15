# Harness AI

The harness this machine's Claude Code and OpenCode installations run: one vendor-neutral policy, the commands and skills that carry the workflow, and the Go CLI that installs and operates them.

- [`harness/README.md`](harness/README.md): the AI workflow, skill supply chain, and validation
- [`harness/POLICY.md`](harness/POLICY.md): the policy linked into each client's global instruction path
- [`AGENTS.md`](AGENTS.md): repository facts, checks, and conventions

## Install

```sh
go run ./cmd/harness bootstrap
```

Builds `harness` into `~/.local/bin` and runs a full install. Requires Go 1.26 or newer and Homebrew.

## Checks

```sh
go vet ./...
test -z "$(gofmt -l .)"
go test ./...
go build ./...
go run ./cmd/harness locks check
```
