# Containers Development Space (CDS)

## Overview

The Containers Development Space (CDS) is a framework designed to build and manage development environment containers, also known as devcontainers. It provides a structured way to organize, build, and run applications within isolated container environments, ensuring consistent behavior across different machines and platforms.

The CDS framework uses a Go-based structure, with application-specific code located in the `cmd/` directory. Each subdirectory under cmd/ represents a separate application, such as the API agent application and the client application. More details about the directory structure can be found in the [Directory Structure](#directory-structure) section.

## Directory Structure

- `.gitignore`: This file is used to exclude certain files from the repository.
- `go.mod`: This is the Go module file. It defines the module’s module path, which is also the import path used for the root directory, and its dependency requirements.
- `go.sum`: This file includes cryptographic checksums of the content of specific module versions.
- `cmd/`: This directory contains application-specific code. Each subdirectory here represents a separate application .
  - `api-agent/`: This directory contains the code for the API agent application.

  - `client/`: This directory contains the code for the client application.
    - `cds.go`: This is the main file for the client application.
- `internal/`: This directory is for code that is not meant to be used by other applications or libraries in the project.
  - `agent/`: This directory contains code related to the agent functionality.
    - `agent.go`: This is the main file for the agent functionality.

## Building and Running

To build and run an application, navigate to its directory under `cmd/` and run `go build` to build the application and `go run` to run it. Alternatively, you can use the `make` command by running `make <appropriate build target>` to build the application and `make <appropriate run target>` to run it, using the [makefile](./makefile) file.

For example, to run the API agent application, navigate to `make run-api-agent`.

### Dependencies

You will need to have `protoc` installed on your system to generate the gRPC code. You can install it by following the instructions [here](https://grpc.io/docs/protoc-installation/).

### Windows

Note that on windows `make` can only be used with Git Bash or WSL.

## Go Linter

To ensure code quality and adherence to coding standards, it is recommended to use a Go linter. A popular Go linter is [golangci-lint](https://golangci-lint.run/).

To run `golangci-lint`, you can use `make lint` command.

## Misc

```bash
❯ go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
❯ go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
❯ protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/api/v1/*.proto

❯ openssl x509 -in ca_cert.pem -text -noout
❯ openssl x509 -in server_cert.pem -text -noout
❯ openssl ec -in server_private_key.pem -text -noout

❯ openssl verify -CAfile ca_cert.pem server_cert.pem
server_cert.pem: OK
```

### TLS

certs keys algorithms: `https://github.dev/cloudflare/cfssl/blob/master/cli/gencert/gencert.go`

### log/slog

- `https://henesgokdag.medium.com/log-slog-library-and-golang-logger-comparision-9e2c3de3d515`
- `https://go.dev/blog/slog`
- `https://github.com/golang/example/blob/master/slog-handler-guide/README.md`
- `https://go.dev/wiki/Resources-for-slog`

### opentelemetry

- https://opentelemetry.io/docs/languages/go/getting-started/
- https://opentelemetry.io/ecosystem/registry/?s=grpc&component=&language=
- https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/instrumentation/google.golang.org/grpc/otelgrpc/interceptor.go
- https://github.com/open-telemetry/opentelemetry-go/tree/main/example

