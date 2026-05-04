---
applyTo: "internal/api/v1/**/*.proto"
---

# CDS Protocol Buffer rules

Protocol Buffer definitions live under `internal/api/v1/` and generate Go code
for the CDS gRPC API.

- Edit the source `.proto` files first; do not hand-edit generated `.pb.go`
  files.
- Preserve package names, import paths, and service boundaries unless the task
  explicitly requires an API change.
- Consider backward compatibility before renaming fields, changing field
  numbers, or changing RPC request/response shapes.
- After changing `.proto` files, run `make build-pb` to regenerate local Go
  output and then update dependent Go code.
- Generated files under `internal/api/v1/cdspb/` are ignored by `.gitignore`.
  Use them for local build/test validation, but do not force-add them unless the
  repository policy changes.
- Add or update tests for new RPC methods, message fields, and validation
  behavior.
