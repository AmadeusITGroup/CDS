---
applyTo: "**/*_test.go"
---

# CDS testing rules

Follow the testing guidance in `CONTRIBUTING.md`.

- Prefer Go's `testing` package with `testify/assert` and `testify/require` for
  new tests.
- Preserve Ginkgo/Gomega style in packages that already use it instead of
  mixing styles within the same test suite.
- Use table-driven tests for multiple scenarios and clear `t.Run` names for
  subcases.
- Use `require` for setup and preconditions that must stop the test; use
  `assert` for independent expectations.
- Mark helper functions with `t.Helper()`.
- Keep tests deterministic. Mock external systems such as HTTP services,
  filesystems, SCM providers, and artifact repositories when possible.
- Use existing fixtures under `test/` or `internal/tests/resources/` before
  adding new fixture structures.
- New code should aim for at least 80% coverage, with emphasis on business
  logic, validation, and error paths.
- Validate with `make test` or a narrower affected `go test ./path/...` command
  before handoff.
