# Contributing to Containers Development Space (CDS)

First off, thank you for considering contributing to CDS! 🎉 It's people like you that make CDS such a great tool for the developer community.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [How to Contribute](#how-to-contribute)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Commit Message Guidelines](#commit-message-guidelines)P
- [Issue Guidelines](#issue-guidelines)
- [Community](#community)

---

## Getting Started

### Prerequisites

Before you begin, ensure you have the following installed:

- **Go 1.26.2+** - [Download Go](https://golang.org/dl/)
- **Protocol Buffers Compiler (protoc)** - [Installation Guide](https://grpc.io/docs/protoc-installation/)
- **Make** - Usually pre-installed on Linux/macOS; use Git Bash or WSL on Windows
- **Git** - [Download Git](https://git-scm.com/)
- **golangci-lint** (recommended) - [Installation](https://golangci-lint.run/usage/install/)

### Fork and Clone

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/CDS.git
   cd CDS
   ```
3. **Add the upstream remote**:
   ```bash
   git remote add upstream https://github.com/AmadeusITGroup/CDS.git
   ```
4. **Keep your fork synced**:
   ```bash
   git fetch upstream
   git checkout main
   git merge upstream/main
   ```

---

## Development Setup

### 1. Install Dependencies

```bash
# Download Go module dependencies
go mod download

# Install Protocol Buffer Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Install golangci-lint (if not already installed)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### 2. Verify Your Setup

```bash
# Run tests to ensure everything is working
make test

# Run linter
make lint

# Build the project
make build
```

### 3. Project Structure Overview

Understanding the project structure will help you contribute effectively:

```
CDS/
├── cmd/                    # Application entry points
│   ├── api-agent/         # API agent service (gRPC server)
│   └── client/            # CDS CLI client
├── internal/              # Private packages (core logic)
│   ├── agent/            # Agent implementation
│   ├── api/v1/           # gRPC API definitions (.proto files)
│   ├── ar/               # Artifactory integration
│   ├── authmgr/          # Authentication management
│   ├── bo/               # Business objects
│   ├── bootstrap/        # Application bootstrapping
│   ├── cenv/             # Environment management
│   ├── cerr/             # Custom error handling
│   ├── clog/             # Logging framework
│   ├── command/          # CLI commands (Cobra)
│   ├── config/           # Configuration (Viper)
│   ├── db/               # Data persistence layer
│   ├── scm/              # Source control management
│   ├── shexec/           # Shell execution utilities
│   ├── systemd/          # Systemd integration (Linux)
│   ├── term/             # Terminal utilities
│   └── tls/              # TLS/certificate management
├── test/                  # Test fixtures and resources
├── makefile               # Build automation
└── go.mod                 # Go module definition
```

---

## How to Contribute

### Types of Contributions

We welcome many types of contributions:

| Type | Description |
|------|-------------|
| 🐛 **Bug Fixes** | Fix issues and improve stability |
| ✨ **New Features** | Add new functionality |
| 📖 **Documentation** | Improve or add documentation |
| 🧪 **Tests** | Add or improve test coverage |
| 🔧 **Refactoring** | Code improvements without changing behavior |
| 🌐 **Translations** | Help translate documentation |
| 💡 **Ideas** | Suggest improvements via issues |

### Contribution Workflow

1. **Find or create an issue** describing the work
2. **Create a feature branch** from `main`
3. **Make your changes** following our coding standards
4. **Write/update tests** for your changes
5. **Run the test suite** and linter
6. **Commit your changes** with meaningful messages
7. **Push to your fork** and create a Pull Request

---

## Pull Request Process

### Before Submitting

Ensure your PR meets these requirements:

- [ ] Code compiles without errors (`make build`)
- [ ] All tests pass (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] New code has appropriate test coverage
- [ ] Documentation is updated if needed
- [ ] Commit messages follow our guidelines

### Creating a Pull Request

1. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/issue-description
   ```

2. **Make your changes** and commit them

3. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

4. **Open a Pull Request** on GitHub with:
   - Clear title describing the change
   - Description of what and why
   - Reference to related issues (e.g., "Fixes #123")
   - Screenshots/examples if applicable

### PR Title Format

Use one of these prefixes:

- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation changes
- `test:` - Adding or updating tests
- `refactor:` - Code refactoring
- `chore:` - Maintenance tasks
- `perf:` - Performance improvements

Example: `feat: add SSH key rotation support`

### Review Process

1. Maintainers will review your PR
2. Address any requested changes
3. Once approved, a maintainer will merge your PR
4. Your contribution will be part of the next release! 🎉

---

## Coding Standards

### Go Style Guidelines

We follow standard Go conventions and best practices:

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Run `gofmt` on all code (handled by linter)
- Keep functions focused and small
- Use meaningful variable and function names

### Package Guidelines

- **`cmd/`**: Only main packages, minimal logic
- **`internal/`**: All business logic goes here
- Keep packages focused on a single responsibility
- Avoid circular dependencies

### Error Handling

Use the project's custom error handling from `internal/cerr`:

```go
import "github.com/amadeusitgroup/cds/internal/cerr"

// Wrap errors with context
if err != nil {
    return cerr.AppendError("failed to process request", err)
}
```

### Logging

Use the project's logging framework from `internal/clog`:

```go
import "github.com/amadeusitgroup/cds/internal/clog"

clog.Info("Processing request", "user", userID)
clog.Error("Failed to connect", err)
clog.Debug("Debug information", "details", data)
```

### Configuration

Use Viper for configuration management:

```go
import "github.com/spf13/viper"

value := viper.GetString("config.key")
```

### gRPC/Protocol Buffers

When modifying `.proto` files in `internal/api/v1/`:

1. Edit the `.proto` file
2. Regenerate Go code:
   ```bash
   make build-pb
   ```
3. Update any dependent code
4. Add tests for new RPC methods

---

## Testing Guidelines

### Test Framework

We use Go's native `testing` package along with the `testify` library for assertions. This provides a simple, standard approach to testing that all Go developers are familiar with.

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make coverage

# Run tests for a specific package
go test ./internal/db/...

# Run tests with verbose output
go test -v ./...

# Run a specific test function
go test -v -run TestProjectCreate ./internal/db/...
```

### Writing Tests

#### Unit Tests

Create test files alongside the code with `_test.go` suffix:

```go
// internal/db/project_test.go
package db

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestProjectCreate(t *testing.T) {
    t.Run("with valid input", func(t *testing.T) {
        project, err := CreateProject("test-project")
        
        require.NoError(t, err)
        assert.Equal(t, "test-project", project.Name)
    })

    t.Run("with empty name returns error", func(t *testing.T) {
        _, err := CreateProject("")
        
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "name cannot be empty")
    })
}
```

#### Table-Driven Tests

Use table-driven tests for testing multiple scenarios:

```go
func TestValidateProjectName(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        wantValid bool
        wantErr   string
    }{
        {
            name:      "valid name",
            input:     "my-project",
            wantValid: true,
        },
        {
            name:      "empty name",
            input:     "",
            wantValid: false,
            wantErr:   "name cannot be empty",
        },
        {
            name:      "name with spaces",
            input:     "my project",
            wantValid: false,
            wantErr:   "name cannot contain spaces",
        },
        {
            name:      "name too long",
            input:     string(make([]byte, 256)),
            wantValid: false,
            wantErr:   "name exceeds maximum length",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateProjectName(tt.input)
            
            if tt.wantValid {
                assert.NoError(t, err)
            } else {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.wantErr)
            }
        })
    }
}
```

#### Test Helpers

Create helper functions to reduce test boilerplate:

```go
// testutils.go (in the same package)
func setupTestDB(t *testing.T) (*Database, func()) {
    t.Helper()
    
    db, err := NewTestDatabase()
    require.NoError(t, err)
    
    cleanup := func() {
        db.Close()
    }
    
    return db, cleanup
}

// Usage in tests
func TestDatabaseOperations(t *testing.T) {
    db, cleanup := setupTestDB(t)
    defer cleanup()
    
    // ... test code
}
```

### Test Coverage

- Aim for **80%+ coverage** on new code
- Focus on testing business logic
- Use table-driven tests for multiple scenarios
- Mock external dependencies (HTTP, filesystem, etc.)
- Use `t.Helper()` in test helper functions for better error reporting

---

## Commit Message Guidelines

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification.

### Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `style` | Formatting, no code change |
| `refactor` | Code refactoring |
| `perf` | Performance improvement |
| `test` | Adding/updating tests |
| `chore` | Maintenance tasks |
| `ci` | CI/CD changes |

### Scopes

Use the package or component name:

- `agent`, `api`, `cli`, `config`, `db`, `scm`, `tls`, etc.

### Examples

```bash
# Feature
feat(cli): add project list command

# Bug fix
fix(agent): resolve connection timeout on slow networks

# Documentation
docs(readme): update installation instructions

# With body and footer
feat(scm): add Bitbucket Server support

Add support for Bitbucket Server in addition to Bitbucket Cloud.
This includes new authentication methods and API endpoints.

Closes #42
```

---

## Issue Guidelines

### Reporting Bugs

When reporting bugs, please include:

1. **Description**: Clear description of the bug
2. **Steps to Reproduce**: Minimal steps to reproduce
3. **Expected Behavior**: What should happen
4. **Actual Behavior**: What actually happens
5. **Environment**:
   - OS and version
   - Go version (`go version`)
   - CDS version
6. **Logs/Screenshots**: If applicable

Use this template:

```markdown
## Bug Description
A clear description of the bug.

## Steps to Reproduce
1. Run `cds project init`
2. Configure X
3. See error

## Expected Behavior
What you expected to happen.

## Actual Behavior
What actually happened.

## Environment
- OS: macOS 14.0
- Go: 1.26.2
- CDS: v1.0.0

## Additional Context
Any other relevant information.
```

### Requesting Features

For feature requests, please include:

1. **Problem**: What problem does this solve?
2. **Solution**: Your proposed solution
3. **Alternatives**: Other solutions you've considered
4. **Additional Context**: Any other information

---

## Community

### Getting Help

- **GitHub Issues**: For bugs and feature requests
- **GitHub Discussions**: For questions and general discussion

### Recognition

Contributors are recognized in:

- Release notes
- The project's README acknowledgments
- GitHub's contributor graph

---

## Quick Reference

| Task | Command |
|------|---------|
| Build project | `make build` |
| Run tests | `make test` |
| Run linter | `make lint` |
| Generate proto | `make build-pb` |
| Run client | `make run-client` |
| Run agent | `make run-api-agent` |
| Tidy dependencies | `make go-tidy` |
| Coverage report | `make coverage` |

---

## License

By contributing to CDS, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).

---

<div align="center">

**Thank you for contributing to CDS!** 💙

Your contributions help make development environments better for everyone.

</div>
