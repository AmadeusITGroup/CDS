# CDS agent instructions

These instructions apply to every automated agent working in this repository.
`CONTRIBUTING.md` is the source of truth; if these notes conflict with it,
follow `CONTRIBUTING.md` and update these instructions.

## Required project rules

- Read `CONTRIBUTING.md` before changing code, tests, documentation, or
  workflows.
- Keep changes focused on the requested task. Do not modify unrelated files or
  clean up unrelated issues.
- Use Go 1.24 or newer, `protoc`, `make`, and Git as described in the
  contributor prerequisites.
- Put application entry-point code under `cmd/` only. Keep business logic in
  `internal/` packages and avoid circular dependencies.
- Use standard Go conventions: `gofmt`, meaningful names, small focused
  functions, and package-level cohesion.
- Wrap errors with context using `internal/cerr` (`AppendError` or
  `AppendErrorFmt`) when returning errors. Do not swallow errors.
- Use `internal/clog` for production logging instead of `fmt.Println`,
  `log.Println`, or ad hoc logging.
- Use Viper for configuration and follow the existing Cobra command patterns
  for CLI changes.
- For `.proto` changes under `internal/api/v1/`, run `make build-pb`, update
  dependent code, and add or update tests. Generated `internal/api/v1/cdspb`
  files are ignored by this repository; regenerate them for validation unless a
  maintainer changes that policy.
- Add or update tests for behavior changes. Prefer Go's `testing` package with
  `testify` for new tests, but preserve Ginkgo/Gomega style in packages that
  already use it.
- Aim for at least 80% coverage on new code and focus tests on business logic
  and edge cases.

## Validation before handoff

Run the most specific checks needed for the files changed, and run the broader
checks before a pull request when practical:

| Change type | Expected validation |
| --- | --- |
| Go code | `make test`, `make lint`, and `make build` when practical |
| Tests only | `make test` or the affected `go test ./path/...` package |
| Protobuf | `make build-pb` plus affected tests/build |
| Dependencies | `make go-tidy`, then tests/build that use the dependency |
| Documentation only | Review rendered Markdown; code checks are not required unless examples changed |

If a required check cannot be run, state the reason and any narrower check that
was run instead.

## Pull requests and commits

- Follow the contribution workflow: create a branch from `main`, make focused
  changes, test and lint, then open a PR.
- Use Conventional Commit messages:
  `<type>(<scope>): <subject>`, with scopes such as `agent`, `api`, `cli`,
  `config`, `db`, `scm`, or `tls`.
- Use PR title prefixes from `CONTRIBUTING.md`: `feat:`, `fix:`, `docs:`,
  `test:`, `refactor:`, `chore:`, or `perf:`.
- In PR descriptions, explain what changed, why it changed, related issues, and
  the commands used to validate the work.
