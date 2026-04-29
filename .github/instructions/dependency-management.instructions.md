---
applyTo: "go.mod,go.sum"
---

# CDS dependency management rules

Follow `CONTRIBUTING.md` and keep dependency changes minimal.

- Add new dependencies only when the standard library or an existing dependency
  is not a good fit.
- Keep `go.mod` on Go 1.24 or newer unless maintainers explicitly choose a
  different toolchain version.
- Run `make go-tidy` after dependency changes.
- Run the affected tests/build that use the changed dependency.
- Do not commit unrelated module churn in `go.mod` or `go.sum`.
