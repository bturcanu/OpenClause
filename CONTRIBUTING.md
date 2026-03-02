# Contributing to OpenClause

Thank you for your interest in OpenClause! This guide covers the process for contributing code, connectors, and documentation.

## Getting Started

### Prerequisites

- Go 1.25+ (see `go.mod`)
- PostgreSQL 16+ (for integration tests)
- Docker (optional, for running services locally)

### Clone & Build

```bash
git clone https://github.com/bturcanu/OpenClause.git
cd OpenClause
go build ./...
go test ./...
```

## Project Layout

```
cmd/                   # Executable entry points
  gateway/             # Main API gateway
  console-api/         # Admin console API
  connector-slack/     # Slack remote connector
  connector-jira/      # Jira remote connector
  connector-template/  # Starter template for new remote connectors
pkg/                   # Shared library code
  connectors/          # Connector types, registry, SDK
    builtins/          # In-process mock connectors
    sdk/               # HTTP handler helper for remote connectors
  approvals/           # Approval workflow engine
  policy/              # Policy evaluation (OPA, risk scoring)
  auth/                # JWT / API-key middleware
  config/              # Configuration helpers
migrations/            # SQL migration files
docs/                  # Extended documentation
```

## Contributing a Connector

Connectors are the primary extension point. See [docs/CONNECTORS.md](docs/CONNECTORS.md) for the full architecture guide.

### Built-in connector (fastest path)

1. Create a new file in `pkg/connectors/builtins/` (e.g. `pagerduty.go`).
2. Implement the `BuiltinConnector` interface:

```go
type BuiltinConnector interface {
    Name() string
    Actions() []string
    Exec(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse
}
```

3. Return deterministic mock data — built-in connectors always run in mock mode.
4. Register your connector in `pkg/connectors/builtins/registry.go` → `All()`.
5. Add a row to the table in `docs/CONNECTORS.md`.

### Remote connector (production-grade)

1. Copy `cmd/connector-template/` to `cmd/connector-yourservice/`.
2. Implement your actions inside the `Exec` method.
3. Support `MOCK_CONNECTORS=true` for deterministic test output.
4. Document required environment variables.
5. Add a Dockerfile if applicable.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- No `panic` in library code — return errors.
- Use `log/slog` for structured logging.
- Keep packages small and focused. Avoid circular imports.
- Exported types and functions require a doc comment.

## Testing

```bash
# Unit tests
go test ./...

# Run a specific package
go test ./pkg/connectors/...

# With race detector
go test -race ./...
```

All connectors should be testable in mock mode without external credentials.

## Pull Request Process

1. **Fork** the repository and create a feature branch from `main`.
2. **Keep PRs focused** — one feature or fix per PR.
3. **Write tests** for new functionality.
4. **Update docs** if you add/change connectors or public APIs.
5. **Run `go build ./...` and `go test ./...`** before submitting.
6. **Describe** your changes clearly in the PR description.

## Commit Messages

Use concise, imperative-mood messages:

```
Add PagerDuty built-in connector
Fix webhook SSRF validation for IPv6
Update connector registry to support ListAll
```

## Reporting Issues

Open a GitHub issue with:

- A clear title
- Steps to reproduce (if applicable)
- Expected vs actual behavior
- Go version and OS

## License

By contributing, you agree that your contributions will be licensed under the same terms as the project.
