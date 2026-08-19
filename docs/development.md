# CDS development workflow

This guide describes the recommended local workflow for contributors. It
complements `README.md` and `CONTRIBUTING.md`; when contribution rules disagree,
`CONTRIBUTING.md` remains authoritative.

## Prerequisites

Install:

- Go version matching `go.mod`
- `protoc`
- `make`
- Git
- Podman or Docker for runtime validation
- SSH client tools for remote agent/container validation

Install the pinned Go-based build tools with:

```sh
make setup-tools
```

This installs the versions configured in the `Makefile` for:

- `golangci-lint`
- `cfssl`
- `cfssljson`
- `protoc-gen-go`
- `protoc-gen-go-grpc`

## Configuration directories

By default, CDS writes CLI configuration under:

```text
$HOME/.xcds
```

The agent uses:

```text
$HOME/.xcds-agent
```

Agent-managed cache, including staged deploy artifacts, lives under
`.xcds-agent/cache`.

For tests and local validation, prefer an isolated config path:

```sh
export CDS_CONFIG_PATH="$HOME/cds-dev-config"
```

When `CDS_CONFIG_PATH` is set, the CLI and agent create `.xcds` or
`.xcds-agent` under that directory. This avoids polluting a developer's normal
CDS state while running tests or experiments.

## Common inner-loop commands

Use these commands while editing:

```sh
make help
make build-fast
make test-fast
go test ./internal/command
go test ./internal/agent
```

`make build-fast` builds `cds` and `cds-api-agent` without regenerating
protobuf, tidying modules, or linting. It is intended for normal edit/compile
feedback after generated files are already up to date.

`make test-fast` runs the packages that usually cover command, agent, database,
container configuration, engine, bootstrap, and SSH execution logic.

## Pre-PR validation

Before opening a PR, run:

```sh
make check
```

`make check` performs the local pre-PR flow:

1. creates the test config/cert directories;
2. generates test certificates;
3. formats Go files;
4. regenerates protobuf Go files;
5. runs `go mod tidy`;
6. runs `golangci-lint`;
7. runs the full Go test suite;
8. builds `cds-api-agent` and `cds`.

For CI parity, also check:

```sh
make build
make test
```

## Protobuf workflow

When editing `.proto` files under `internal/api/v1`, run:

```sh
make build-pb
go test ./internal/agent ./internal/command
```

Generated files under `internal/api/v1/cdspb` are regenerated for validation but
are ignored by the repository.

## Local agent workflow

Build the binaries:

```sh
make build-fast
```

Use an isolated config path and ensure the built binaries are on `PATH`:

```sh
export CDS_CONFIG_PATH="$HOME/cds-local-dev"
export PATH="$PWD:$PATH"
./cds space host add localhost
./cds space host get localhost
```

The local agent bootstrap expects `cds-api-agent` to be available on `PATH`.

## Runtime validation with a test flavour

Substitute `my-flavour/latest` below with the path to your own test flavour.

On macOS with Podman, use HOME-based paths because the Podman VM may not
share arbitrary `/tmp` paths:

```sh
export CDS_CONFIG_PATH="$HOME/cds-local-validation"
export PATH="$PWD:$PATH"

mkdir -p "$HOME/workspace" "$HOME/m2repo" "$HOME/.devbox"

./cds space host add localhost
./cds project run --path my-flavour/latest --target localhost
./cds project sync latest
./cds project ssh latest
./cds project clear latest
./cds project delete latest
```

## Remote validation workflow

Remote validation currently requires a running remote agent. Until remote agent
bootstrap is fully automated, the reliable manual flow is:

```sh
GOOS=linux GOARCH=amd64 go build -o cds-api-agent-linux ./cmd/api-agent/cds-api-agent.go

REMOTE_HOST=my-remote-host.example.com
REMOTE_DIR=cds-remote-validation
LOCAL_CFG="$HOME/cds-remote-validation"

export CDS_CONFIG_PATH="$LOCAL_CFG"
export PATH="$PWD:$PATH"

mkdir -p "$LOCAL_CFG/.xcds/certs" "$LOCAL_CFG/.xcds-agent/certs"
make gencert

ssh "$REMOTE_HOST" "rm -rf ~/$REMOTE_DIR && mkdir -p ~/$REMOTE_DIR/.xcds-agent/certs"
scp cds-api-agent-linux "$REMOTE_HOST:~/$REMOTE_DIR/cds-api-agent"
scp "$LOCAL_CFG/.xcds-agent/certs/"* "$REMOTE_HOST:~/$REMOTE_DIR/.xcds-agent/certs/"
ssh "$REMOTE_HOST" \
  "CDS_CONFIG_PATH=\$HOME/$REMOTE_DIR nohup ~/$REMOTE_DIR/cds-api-agent > ~/$REMOTE_DIR/agent.log 2>&1 < /dev/null &"

./cds space host add "$REMOTE_HOST"
./cds space host get "$REMOTE_HOST"
./cds project run --path my-flavour/latest --target "$REMOTE_HOST"
```

Clean up afterwards:

```sh
./cds project clear latest
./cds project delete latest
ssh "$REMOTE_HOST" "rm -rf ~/$REMOTE_DIR"
rm -f cds-api-agent-linux
```

## Suggested next DX improvements

- Add `cds doctor` to check tools, certs, config paths, Podman, SSH keys, and
  agent reachability.
- Add first-class remote agent bootstrap/status/log commands.
- Add integration test tags for local and remote runtime validation.
- Add protobuf lint and breaking-change checks.
