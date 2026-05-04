# Copilot instructions for CDS

Follow `AGENTS.md` and `CONTRIBUTING.md` for all work in this repository.
`CONTRIBUTING.md` remains authoritative when instructions disagree.

Before editing, identify the relevant scoped instruction files in
`.github/instructions/`:

- `contributing-workflow.instructions.md` for repository-wide contribution,
  validation, PR, and commit rules.
- `go-code.instructions.md` for Go implementation changes.
- `testing.instructions.md` for test files and test strategy.
- `protobuf.instructions.md` for gRPC and Protocol Buffer changes.
- `dependency-management.instructions.md` for `go.mod` and `go.sum` changes.

Keep changes surgical, update tests and documentation when behavior changes, and
report any validation that could not be run. Do not bypass the contributing
rules to make a task appear complete.
