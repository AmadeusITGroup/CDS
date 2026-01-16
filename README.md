# Containers Development Space (CDS)

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go)](go.mod)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

> A powerful framework for building and managing development environment containers with consistent, reproducible workflows across teams and platforms.

---

## 📋 Table of Contents

- [Containers Development Space (CDS)](#containers-development-space-cds)
  - [📋 Table of Contents](#-table-of-contents)
  - [🎯 Overview](#-overview)
    - [Why CDS?](#why-cds)
  - [✨ Features](#-features)
  - [📦 Prerequisites](#-prerequisites)
    - [Optional Tools](#optional-tools)
  - [🚀 Installation](#-installation)
    - [From Source](#from-source)
  - [⚡ Quick Start](#-quick-start)
    - [Running the CDS Client](#running-the-cds-client)
    - [Running the API Agent](#running-the-api-agent)
    - [Generate TLS Certificates](#generate-tls-certificates)
    - [Run Tests](#run-tests)
  - [📖 Usage](#-usage)
    - [Client Commands](#client-commands)
    - [Configuration](#configuration)
    - [Working with Projects](#working-with-projects)
    - [Certificate Management](#certificate-management)
  - [🏗️ Project Structure](#️-project-structure)
    - [Key Directories](#key-directories)
  - [🛠️ Development](#️-development)
    - [Building from Source](#building-from-source)
    - [Generating Protocol Buffers](#generating-protocol-buffers)
    - [Code Quality](#code-quality)
    - [Dependency Management](#dependency-management)
    - [Platform-Specific Notes](#platform-specific-notes)
      - [Windows](#windows)
      - [Linux](#linux)
      - [macOS](#macos)
  - [🤝 Contributing](#-contributing)
    - [Getting Started](#getting-started)
    - [Development Guidelines](#development-guidelines)
    - [Code Style](#code-style)
    - [Testing](#testing)
    - [Reporting Issues](#reporting-issues)
  - [📄 License](#-license)
  - [🙏 Acknowledgments](#-acknowledgments)
    - [References \& Resources](#references--resources)
  - [📞 Support](#-support)


---

## 🎯 Overview

**Containers Development Space (CDS)** is a Go-based framework designed to streamline the creation, management, and orchestration of development environment containers (devcontainers). CDS provides a structured approach to building consistent development environments that work seamlessly across different machines, operating systems, and team configurations.

### Why CDS?

- **Consistency**: Ensure all developers work in identical environments
- **Portability**: Development environments that work on Linux, macOS, and Windows
- **Security**: Built-in TLS/SSL support with certificate management
- **Integration**: Native support for Git, Artifactory, and Bitbucket
- **Extensibility**: Modular architecture with gRPC-based APIs

---

## ✨ Features

- 🐳 **Container Orchestration**: Build and manage development containers with ease
- 🔐 **Secure Communication**: Built-in TLS/SSL certificate generation and management
- 🌐 **gRPC API**: High-performance API for agent-based communication
- 🔄 **SCM Integration**: Native support for Git, Bitbucket, and other version control systems
- 📦 **Artifact Management**: Integration with JFrog Artifactory
- 🖥️ **Cross-Platform**: Support for Linux, macOS, and Windows
- 🔧 **Systemd Integration**: Native systemd support for Linux environments
- 📊 **Structured Logging**: Advanced logging with Go's log/slog
- 🧪 **Testing Framework**: Comprehensive test suite using Ginkgo and Gomega
- 🎨 **Rich CLI**: Beautiful terminal output with pterm

---

## 📦 Prerequisites

Before installing CDS, ensure you have the following dependencies:

- **Go**: Version 1.24.0 or higher ([Download](https://golang.org/dl/))
- **Protocol Buffers Compiler**: protoc for gRPC code generation ([Installation Guide](https://grpc.io/docs/protoc-installation/))
- **Make**: Build automation tool
  - Linux/macOS: Usually pre-installed
  - Windows: Use [Git Bash](https://git-scm.com/downloads) or [WSL](https://docs.microsoft.com/en-us/windows/wsl/install)
- **OpenSSL**: For TLS certificate operations (usually pre-installed on Linux/macOS)

### Optional Tools

- **golangci-lint**: For code quality checks ([Installation](https://golangci-lint.run/usage/install/))

---

## 🚀 Installation

### From Source

1. **Clone the repository**:
   ```bash
   git clone https://github.com/AmadeusITGroup/CDS.git
   cd CDS
   ```

2. **Install dependencies**:
   ```bash
   go mod download
   ```

3. **Install Protocol Buffer tools**:
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```

4. **Build the project**:
   ```bash
   make install
   ```

---

## ⚡ Quick Start

### Running the CDS Client

```bash
make run-client
```

### Running the API Agent

```bash
make run-api-agent
```

### Generate TLS Certificates

```bash
make gencert
```

### Run Tests

```bash
make test
```

---

## 📖 Usage

### Client Commands

The CDS client provides several commands for managing your development spaces:

```bash
# Initialize a new project
cds project init

# Initialize a new space
cds space init

# Check version
cds version
```

### Configuration

CDS configuration is stored in ~/.cds/ directory. You can customize settings through:
- Configuration files
- Environment variables
- Command-line flags

### Working with Projects

```bash
# Create a new project scaffold
make scaffold
```

### Certificate Management

Generate and manage TLS certificates for secure communication:

```bash
# Verify certificates
openssl verify -CAfile ca_cert.pem server_cert.pem

# Inspect certificate details
openssl x509 -in server_cert.pem -text -noout
```

---

## 🏗️ Project Structure

```
CDS/
├── cmd/                    # Application entry points
│   ├── api-agent/         # API agent service
│   └── client/            # CDS CLI client
├── internal/              # Private application code
│   ├── agent/            # Agent implementation
│   ├── api/              # gRPC API definitions
│   ├── ar/               # Artifactory integration
│   ├── authmgr/          # Authentication management
│   ├── bo/               # Business objects
│   ├── bootstrap/        # Application bootstrapping
│   ├── cenv/             # Environment management
│   ├── cerr/             # Error handling
│   ├── clog/             # Logging framework
│   ├── command/          # CLI commands
│   ├── config/           # Configuration management
│   ├── db/               # Database/storage layer
│   ├── host/             # Host management
│   ├── profile/          # Profile management
│   ├── scm/              # Source control management
│   ├── shexec/           # Shell execution utilities
│   ├── systemd/          # Systemd integration
│   ├── term/             # Terminal utilities
│   └── tls/              # TLS/certificate management
├── test/                  # Test resources
├── go.mod                 # Go module definition
├── makefile               # Build automation
└── LICENSE                # Apache 2.0 License
```

### Key Directories

- **cmd/**: Contains the main applications. Each subdirectory is a separate executable.
- **internal/**: Private packages not intended for external import. Contains the core business logic.
- **test/**: Test fixtures, resources, and integration tests.

---

## 🛠️ Development

### Building from Source

```bash
# Build all binaries
make build

# Build specific components
make build-client
make build-api-agent
```

### Generating Protocol Buffers

When modifying .proto files:

```bash
make build-pb
```

Or manually:

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       internal/api/v1/*.proto
```

### Code Quality

```bash
# Run linter
make lint

# Run linter with auto-fix
make lint-weak

# Run tests
make test

# Generate coverage report
make coverage
```

### Dependency Management

```bash
# Tidy dependencies
make go-tidy
```

### Platform-Specific Notes

#### Windows
- Use Git Bash or Windows Subsystem for Linux (WSL) to run make commands
- Ensure paths are properly escaped when working with Windows paths

#### Linux
- Systemd integration is available for service management
- Check systemd service files in internal/systemd/

#### macOS
- Boot configuration available in internal/bootstrap/boot_darwin.go

---

## 🤝 Contributing

We welcome contributions from the community! Here's how you can help:

### Getting Started

1. **Fork the repository**
2. **Create a feature branch**: git checkout -b feature/amazing-feature
3. **Make your changes**
4. **Run tests**: make test
5. **Run linter**: make lint
6. **Commit your changes**: git commit -m 'Add amazing feature'
7. **Push to the branch**: git push origin feature/amazing-feature
8. **Open a Pull Request**

### Development Guidelines

- Follow Go best practices and idioms
- Write comprehensive tests for new features
- Update documentation for API changes
- Ensure all tests pass before submitting PR
- Keep commits atomic and well-described

### Code Style

This project uses golangci-lint to enforce code quality. Run the linter before submitting:

```bash
make lint
```

### Testing

- Write unit tests for new functionality
- Update integration tests when changing APIs
- Aim for high test coverage

### Reporting Issues

Found a bug? Have a feature request? Please open an issue with:
- Clear description of the problem
- Steps to reproduce (for bugs)
- Expected vs actual behavior
- Environment details (OS, Go version, etc.)

---

## 📄 License

This project is licensed under the **Apache License 2.0** - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

Built with these excellent open-source projects:

- [Go](https://golang.org/) - The Go Programming Language
- [gRPC](https://grpc.io/) - High-performance RPC framework
- [Protocol Buffers](https://developers.google.com/protocol-buffers) - Data serialization
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [Ginkgo](https://github.com/onsi/ginkgo) & [Gomega](https://github.com/onsi/gomega) - Testing framework
- [pterm](https://github.com/pterm/pterm) - Beautiful terminal output
- [go-git](https://github.com/go-git/go-git) - Git implementation in Go

### References & Resources

- **TLS/Certificate Management**: Inspired by [Cloudflare CFSSL](https://github.com/cloudflare/cfssl)
- **Logging**: Built with Go's [log/slog](https://go.dev/blog/slog) - [Guide](https://github.com/golang/example/blob/master/slog-handler-guide/README.md)
- **Observability**: [OpenTelemetry for Go](https://opentelemetry.io/docs/languages/go/getting-started/)
- **gRPC Instrumentation**: [OpenTelemetry gRPC](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/google.golang.org/grpc/otelgrpc)

---

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/AmadeusITGroup/CDS/issues)
- **Discussions**: [GitHub Discussions](https://github.com/AmadeusITGroup/CDS/discussions)

---

<div align="center">
Made with ❤️ by the CDS community
</div>
