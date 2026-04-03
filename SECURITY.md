# Security Policy

## Reporting Vulnerabilities

If you discover a security vulnerability in cb365, **do not open a public issue**.

Instead, please email **security@cb365.dev** with:

- Description of the vulnerability
- Steps to reproduce
- Impact assessment
- Any suggested fix

You will receive an acknowledgement within 48 hours and a detailed response within 7 days.

## Security Design Principles

### Token Storage
- All authentication tokens are stored in the operating system's native keychain (macOS Keychain, Linux secret-service/libsecret, Windows Credential Manager).
- Tokens are **never** stored in plaintext files.
- Tokens are **never** logged, even at verbose/debug log levels.

### Authentication
- cb365 uses Microsoft's official `azidentity` library for all Entra ID authentication flows.
- No custom cryptographic implementations.
- OAuth 2.0 device code flow for delegated authentication.
- Client credentials flow for unattended/service authentication.

### Least Privilege
- Each workload module requests only the Graph API scopes it needs.
- The `--dry-run` flag allows previewing write operations without executing them.
- Write operations in non-interactive mode require explicit `--force` flag.

### Supply Chain
- Minimal dependency tree — Go standard library + Microsoft official SDKs.
- `govulncheck` runs in CI on every commit.
- Software Bill of Materials (SBOM) attached to every release.
- All release binaries are signed.

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.x     | ✓ (current development) |

## Scope

This policy covers the cb365 CLI tool and its source code. It does not cover the Microsoft Graph API, Entra ID, or any Microsoft services that cb365 interacts with.
