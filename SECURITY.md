# Security Policy

## Reporting Vulnerabilities

If you discover a security vulnerability in cb365, **do not open a public issue**.

Instead, please use [GitHub Security Advisories](https://github.com/nz365guy/cb365/security/advisories/new) to report it privately. Include:

- Description of the vulnerability
- Steps to reproduce
- Impact assessment
- Any suggested fix

You will receive an acknowledgement within 48 hours and a detailed response within 7 days.

## Security Design Principles

### Token Storage
- Managed delegated bearer material is stored only in a profile-bound Bitwarden Secrets Manager EU record.
- App-only credentials use the operating system keychain, with a profile-bound AES-256-GCM encrypted-file fallback for headless systems.
- Tokens are **never** stored in plaintext files.
- Tokens are **never** logged, even at verbose/debug log levels.

### Authentication
- cb365 uses Microsoft's official `azidentity` library for all Entra ID authentication flows.
- The encrypted-file fallback uses Go's AES-GCM implementation and binds each ciphertext to its profile name.
- OAuth 2.0 device code flow for delegated authentication.
- Client credentials flow for unattended/service authentication.

### Private Input
- String flags support `@path` and `@-` indirect input so business content does not need to appear in process arguments.
- Use indirect input for subjects, message bodies, names, addresses, queries, tenant paths, and list values.
- Treat indirect input files as sensitive, restrict their permissions, and remove them according to local policy.
- Authentication secrets use the dedicated stdin/certificate flow and must never be placed in a general content flag.

### Least Privilege
- Each workload module requests only the Graph API scopes it needs.
- The `--dry-run` flag allows previewing write operations without executing them.
- Destructive writes require `--force`; external communications require `--confirm`; all writes support `--dry-run` where documented.

### Supply Chain
- Dependencies are locked through `go.mod`/`go.sum` and reviewed by CI.
- `gosec`, `govulncheck`, and secret scanning run in CI.
- Release workflows generate an SBOM and sign release artifacts when a release is published successfully.

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.x     | ✓ (current development) |

## Scope

This policy covers the cb365 CLI tool and its source code. It does not cover the Microsoft Graph API, Entra ID, or any Microsoft services that cb365 interacts with.
