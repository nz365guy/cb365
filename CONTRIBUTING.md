# Contributing to cb365

Thanks for your interest in contributing to cb365. This document covers the process for submitting changes.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/cb365.git`
3. Create a branch: `git checkout -b feat/your-feature`
4. Make your changes
5. Run the checks (see below)
6. Submit a pull request

## Development Requirements

- **Go 1.24+**
- **gosec** — `go install github.com/securego/gosec/v2/cmd/gosec@latest`
- **govulncheck** — `go install golang.org/x/vuln/cmd/govulncheck@latest`

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use `cmd/cb365/{workload}.go` for new workload modules — one file per workload
- Use `cmd/cb365/{workload}_test.go` for tests — one test file per workload
- Internal packages go in `internal/` — these are not part of the public API
- Keep the dependency tree minimal — prefer the Go standard library

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add planner tasks create command
fix: calendar timezone validation for UTC offsets
docs: update README with planner examples
test: add mail send safety rule tests
ci: enable govulncheck in CI pipeline
deps: bump msgraph-sdk-go to v1.97.0
```

## Before Submitting a PR

Every PR must pass these checks locally:

```bash
# Build
go build -v ./cmd/cb365/

# Tests
go test -v ./...

# Vet
go vet ./...

# Security scan
gosec ./...

# Vulnerability check
govulncheck ./...
```

CI runs all of these automatically on every push and PR.

## Quality Gates

All contributions must meet these requirements:

| Gate | Requirement |
|------|-------------|
| Build | `go build` succeeds on Linux, macOS, and Windows |
| Tests | `go test ./...` passes with no failures |
| Security | `gosec` reports zero high/critical issues |
| Vulnerabilities | `govulncheck` reports zero issues in project code |
| Token safety | No tokens, secrets, or credentials appear in output at any verbosity |
| Dry-run | Every write command supports `--dry-run` and it is tested |
| JSON output | Every command produces valid JSON with `--json` |
| Output design | Human messages on stderr, machine output on stdout |

## Adding a New Workload

If you're adding a new Microsoft 365 workload:

1. Create `cmd/cb365/{workload}.go` with the command tree
2. Create `cmd/cb365/{workload}_test.go` with unit tests
3. Register the root command in `cmd/cb365/root.go`
4. Follow the established patterns:
   - `--dry-run` on all write operations
   - `--force` on destructive operations (delete, overwrite)
   - `--confirm` on broadcast operations
   - Name-to-ID resolution where applicable
   - All three output formats (table, JSON, plain)
5. Add the required Entra scopes to the README
6. Add safety rules appropriate to the workload

## Safety Rules

Safety rules are hardcoded in Go — not configurable, not bypassable. If you add a write command, you must add safety rules. At minimum:

- `--dry-run` support
- `--force` for destructive operations
- Input validation before calling the Graph API
- Audit metadata where applicable (e.g., `[Sent via cb365]` on outbound messages)

## Security Considerations

- **Never log tokens or secrets** — not in verbose mode, not in error messages, not in test output
- **Use `azidentity` for auth** — no custom OAuth implementations
- **Use `msgraph-sdk-go` for Graph calls** — no raw HTTP to Graph endpoints
- **Validate before calling the API** — catch problems before they reach Microsoft's servers
- **File operations use temp files** — download to temp, validate, then move to final path

## Reporting Security Issues

Do **not** open a public issue for security vulnerabilities. See [SECURITY.md](SECURITY.md) for responsible disclosure instructions.

## Licence

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).

