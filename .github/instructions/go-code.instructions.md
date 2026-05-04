---
applyTo: "**/*.go"
---

# CDS Go implementation rules

Follow standard Go conventions and the project-specific rules from
`CONTRIBUTING.md`.

## Structure

- Use Go 1.24 or newer.
- Keep `cmd/` packages as thin application entry points with minimal logic.
- Place business logic in focused `internal/` packages.
- Avoid circular dependencies and keep packages responsible for one concern.
- Run `gofmt` on Go changes; rely on `make lint` for project lint checks.

## Error handling and logging

- Return errors instead of ignoring them. Do not add silent fallbacks or broad
  error suppression.
- When adding context to returned errors, use `internal/cerr`:
  `cerr.AppendError("message", err)` or `cerr.AppendErrorFmt("message %s", err,
  value)`.
- Use `internal/clog` for production logs:
  `clog.Debug`, `clog.Info`, or `clog.Error`.
- Avoid `fmt.Println`, `log.Println`, and direct stdout/stderr writes in
  production paths unless the existing CLI command pattern requires terminal
  output.

## Project conventions

- Use Viper for configuration access and keep configuration keys consistent with
  existing packages.
- Follow the existing Cobra command patterns for CLI additions under
  `internal/command` and thin launchers under `cmd/`.
- Keep public APIs and exported identifiers documented when Go linting or
  package clarity requires it.
- Add tests for new behavior and edge cases in the same package or an existing
  package-level test structure.
