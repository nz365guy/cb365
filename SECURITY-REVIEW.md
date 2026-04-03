# Security Review Guide

> **Scope:** `internal/auth/` — 648 lines across 3 files
> **Estimated time:** 2–4 hours
> **Purpose:** Independent review before open-sourcing this repository

---

## What Is cb365?

A Go CLI for Microsoft 365 via Microsoft Graph. It handles Entra ID authentication, stores tokens in the OS keychain (or AES-256-GCM encrypted files on headless Linux), and provides 58 commands across 10 M365 workloads. The auth module is the most security-sensitive code in the project.

## Review Scope

Three files — start here:

| File | Lines | What It Does |
|------|-------|-------------|
| [`internal/auth/auth.go`](internal/auth/auth.go) | 287 | Token acquisition: device-code flow, client credentials, certificate auth. JWT decode for display (never logs raw tokens). |
| [`internal/auth/keyring.go`](internal/auth/keyring.go) | 162 | OS keychain abstraction via `go-keyring`. Probes keychain availability on startup. Falls back to encrypted file if no keychain. |
| [`internal/auth/store_file.go`](internal/auth/store_file.go) | 199 | AES-256-GCM encrypted file storage for headless Linux. PBKDF2 key derivation from environment variable passphrase. |

Everything else in the repo (commands, output formatting, Graph API calls) is out of scope for this review.

## Dependencies

The auth module depends on:

| Package | Version | Role |
|---------|---------|------|
| `azidentity` v1.13.1 | Microsoft's official Entra ID library | All OAuth flows |
| `go-keyring` v0.2.8 | OS-native keychain (macOS/Windows/Linux) | Token storage (primary) |
| `golang.org/x/crypto` v0.47.0 | PBKDF2 key derivation | Encrypted file fallback |
| Go stdlib `crypto/aes`, `crypto/cipher` | AES-256-GCM | Encrypted file fallback |

No custom OAuth. No custom cryptographic primitives.

## What to Verify

### 1. Tokens never stored in plaintext

All tokens pass through OS keychain or AES-256-GCM encrypted file. There should be no path that writes a raw token to an unencrypted file, environment variable cache, or config JSON.

**Files:** `keyring.go` (primary path), `store_file.go` (fallback)

### 2. Tokens never appear in output

Raw access tokens, refresh tokens, and client secrets should never appear in stdout, stderr, or log output at any verbosity level. The `auth.go` file decodes JWT claims for display but should never log the raw token string.

**Files:** `auth.go` — trace all `fmt.Print*`, `log.*` calls

### 3. Client secrets stored encrypted

For app-only auth, the client secret is stored alongside the access token using the same keychain/encrypted-file storage. It should never appear in config files or log output.

**Files:** `auth.go` `LoginAppOnly()`, `RefreshAppOnly()`

### 4. AES-256-GCM implementation is sound

The encrypted file fallback in `store_file.go` uses AES-256-GCM with PBKDF2-derived keys. Specific things to check:

- PBKDF2 iteration count (currently 210,000 — is this sufficient?)
- Salt generation (should be random, from `crypto/rand`)
- Nonce uniqueness (should be fresh per encryption, from `crypto/rand`)
- File permissions (should be 0600)
- Atomic write pattern (should use temp file + rename)

### 5. Keyring probe is safe

On startup, `keyring.go` probes the OS keychain with a test write/read/delete cycle. Check whether this could leak data or leave artifacts on failure.

### 6. Certificate private key handling

Certificate auth in `auth.go` `LoginCertificate()` reads a PEM file and passes the private key to `azidentity.NewClientCertificateCredential`. Check whether the private key material persists in memory longer than necessary or could appear in error messages.

## How to Review

```bash
git clone https://github.com/nz365guy/cb365.git
cd cb365

# Read the three files
cat internal/auth/auth.go
cat internal/auth/keyring.go
cat internal/auth/store_file.go

# Run existing tests
go test -v ./...

# Run security scanners
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...

go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## Known Accepted Risks

These are documented and do not need to be reported:

1. **No MSAL token cache serialisation** — delegated tokens expire after ~1 hour with no automatic refresh. This is a UX trade-off, not a security issue.
2. **`CB365_KEYRING_PASSWORD` in environment** — the encrypted file passphrase must be available at runtime. On headless servers this means `.bashrc` or a secret manager.
3. **Go stdlib vulnerabilities GO-2026-4601 and GO-2026-4602** — known issues requiring Go 1.25, which is not yet released. Tracked in CI.

## Deliverable

A brief written report covering:

1. **Findings** — any issues found, rated by severity (critical / high / medium / low / informational)
2. **Verification** — confirmation of the 6 checks above (verified / not verified / concern)
3. **Recommendations** — suggested improvements, if any

A few paragraphs is fine. This is a focused micro-audit, not a penetration test.

Thank you for taking the time to review this.

