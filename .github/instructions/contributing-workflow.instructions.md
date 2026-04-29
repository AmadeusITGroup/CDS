---
applyTo: "**"
---

# CDS contribution workflow

Treat `CONTRIBUTING.md` as the source of truth for contribution rules. These
instructions summarize the parts that every working agent must apply.

## Scope and workflow

- Work on a task branch, keep the diff focused, and do not touch unrelated
  files.
- Preserve existing behavior unless the task explicitly asks for a behavior
  change.
- Update documentation when user-facing behavior, setup steps, commands, or
  configuration change.
- Do not commit generated build artifacts such as `cds`, `cds-api-agent`,
  `coverage.out`, certificates, or delivery archives.

## Required validation

Choose validation based on the files changed:

| Files changed | Validation |
| --- | --- |
| Go source | `make test`, `make lint`, and `make build` when practical |
| Go tests | `make test` or the affected `go test ./path/...` package |
| `.proto` files | `make build-pb` and affected tests/build |
| `go.mod` or `go.sum` | `make go-tidy` and affected tests/build |
| Docs only | Review Markdown; code checks are optional unless examples changed |

If a check cannot be run, say why and include the best narrower validation that
was possible.

## PR and commit rules

- Use Conventional Commits:
  `<type>(<scope>): <subject>`.
- Valid commit types include `feat`, `fix`, `docs`, `style`, `refactor`,
  `perf`, `test`, `chore`, and `ci`.
- Prefer scopes from the project components, such as `agent`, `api`, `cli`,
  `config`, `db`, `scm`, or `tls`.
- Use PR title prefixes from `CONTRIBUTING.md`: `feat:`, `fix:`, `docs:`,
  `test:`, `refactor:`, `chore:`, or `perf:`.
- PR descriptions must explain what changed, why it changed, related issues,
  and how the change was validated.
