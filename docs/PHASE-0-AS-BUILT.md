# Phase 0 — As-Built Documentation

> **Phase:** 0 — Foundation
> **Status:** Scaffold complete, pending live auth test
> **Date:** 2026-04-03
> **Author:** Mark Smith / Claude (Tier 4 MCP session)

---

## 1. Overview

Phase 0 establishes the foundational architecture for cb365: the CLI framework, Entra ID authentication, secure token storage, profile management, and output formatting. No Microsoft Graph workload commands are included — this phase is purely the chassis that all subsequent phases build on.

## 2. Architecture

### 2.1 High-Level Design

```
┌──────────────────────────────────────────────────────┐
│                      cb365 CLI                        │
│                                                       │
│  cmd/cb365/          ← Command definitions (cobra)    │
│    main.go           ← Entrypoint                     │
│    root.go           ← Root command + global flags    │
│    auth.go           ← auth subcommands               │
│                                                       │
│  internal/           ← Private implementation         │
│    auth/             ← Entra ID auth + token mgmt     │
│    config/           ← Profile & settings persistence │
│    output/           ← JSON/plain/table formatters    │
└───────────┬──────────────────────┬────────────────────┘
            │                      │
            ▼                      ▼
   ┌────────────────┐    ┌─────────────────┐
   │   azidentity   │    │   go-keyring    │
   │  (Microsoft)   │    │  (OS keychain)  │
   └───────┬────────┘    └────────┬────────┘
           │                      │
           ▼                      ▼
   ┌────────────────┐    ┌─────────────────┐
   │  Entra ID      │    │  macOS Keychain │
   │  (login.ms)    │    │  secret-service │
   │                │    │  wincred        │
   └────────────────┘    └─────────────────┘
```

### 2.2 Design Principles

1. **Tokens never touch disk in plaintext.** All token storage uses the operating system's native credential manager via `go-keyring`. Configuration files at `~/.config/cb365/config.json` contain profile metadata (tenant ID, client ID, auth mode) but never tokens or secrets.

2. **Tokens never appear in output.** The `TokenInfo` struct extracts display-safe JWT claims (UPN, name, scopes, expiry) but never includes the raw access token. The `--verbose` flag increases logging detail but is explicitly designed to exclude secrets.

3. **Agent-first output.** Every command supports `--json` for structured machine-readable output written to stdout. Human-readable messages (success, info, error) are written to stderr so that `cb365 auth status --json | jq .` works cleanly in pipelines.

4. **Profile isolation.** Multiple Entra tenants and auth modes can coexist as named profiles. Only one profile is active at a time, selectable via `--profile` flag or `cb365 auth use`.

## 3. Component Detail

### 3.1 CLI Framework — `cmd/cb365/`

**Framework:** [spf13/cobra](https://github.com/spf13/cobra) v1.10.2

**Command tree:**

```
cb365
├── auth
│   ├── login       — Entra ID device-code authentication
│   ├── status      — Display current token info (no secrets)
│   ├── logout      — Revoke and clear cached credentials
│   ├── profiles    — List all configured profiles
│   └── use         — Switch active profile
├── version         — Print version string
└── help            — Auto-generated help
```

**Global flags (defined in `root.go`):**

| Flag | Type | Default | Purpose |
|------|------|---------|---------|
| `--json` | bool | false | JSON output to stdout |
| `--plain` | bool | false | Tab-separated output to stdout |
| `--profile` | string | "" | Override active profile for this command |
| `--verbose` | bool | false | Verbose logging (never includes secrets) |
| `--dry-run` | bool | false | Preview write operations without executing |

**Entry point (`main.go`):** Minimal — calls `rootCmd.Execute()` and exits non-zero on error. Cobra handles all command routing.

### 3.2 Authentication — `internal/auth/`

#### 3.2.1 `auth.go` — Entra ID Flows

**Device-code flow (`LoginDelegated`):**

1. Creates `azidentity.DeviceCodeCredential` with the profile's tenant and client IDs
2. Prints the device-code URL and user code to stderr via the `UserPrompt` callback
3. User visits the URL, enters the code, and authenticates
4. `GetToken()` returns an `azcore.AccessToken` containing the access token string and expiry time
5. Caller stores the token in the OS keychain

**Scope handling (`GraphScopes`):** Short scope names like `Tasks.ReadWrite` are automatically expanded to full URIs (`https://graph.microsoft.com/Tasks.ReadWrite`). If no scopes are provided, falls back to `https://graph.microsoft.com/.default`.

**JWT decode (`DecodeTokenInfo`):** Extracts display-safe claims from the access token JWT payload. This is for informational display only — it does NOT validate the token signature (that's the Graph API's job). Extracted fields:

| Claim | Field | Source JWT Claim |
|-------|-------|-----------------|
| User principal name | `UPN` | `upn` |
| Display name | `Name` | `name` |
| Tenant ID | `TenantID` | `tid` |
| Application name | `AppName` | `app_displayname` |
| Scopes | `Scopes` | `scp` (space-separated) |
| Expiry | `ExpiresAt` | `exp` (Unix timestamp → RFC3339) |
| Time remaining | `ValidFor` | Calculated from `exp` |
| Expired flag | `IsExpired` | Calculated from `exp` |

#### 3.2.2 `keyring.go` — Secure Token Storage

**Backend:** `zalando/go-keyring` v0.2.8

**Storage format:** JSON-serialised `TokenCache` struct stored as a single keyring entry per profile.

```
Service:  "cb365"
Key:      <profile-name>  (e.g. "work-delegated")
Value:    {"access_token":"eyJ...","expires_at":"2026-...","token_type":"Bearer","scope":"..."}
```

**Platform backends:**

| OS | Backend | Notes |
|----|---------|-------|
| macOS | Keychain | Native, works headless |
| Linux (desktop) | `secret-service` (D-Bus) | Requires `gnome-keyring` or `kwallet` |
| Linux (headless) | **⚠️ NOT YET SOLVED** | Falls back to failure — needs Phase 0 completion |
| Windows | Windows Credential Manager | Native |

**Operations:**

| Function | Behaviour |
|----------|-----------|
| `StoreToken(profile, cache)` | Serialises to JSON, stores in keyring |
| `LoadToken(profile)` | Retrieves from keyring, deserialises |
| `DeleteToken(profile)` | Removes from keyring, no-op if not found |

**Security note:** The `TokenCache` struct comment explicitly marks it as containing secrets and warns against logging or printing it.

### 3.3 Configuration — `internal/config/`

**File location:** `~/.config/cb365/config.json`
**Permissions:** Created with `0700` (directory) and `0600` (file)

**Schema:**

```json
{
  "active_profile": "work-delegated",
  "profiles": {
    "work-delegated": {
      "name": "work-delegated",
      "tenant_id": "0ab1b54d-...",
      "client_id": "22809b82-...",
      "auth_mode": "delegated",
      "scopes": ["Tasks.ReadWrite", "Mail.Read"],
      "username": "mark.smith@cloverbase.com",
      "active": true
    }
  },
  "settings": {
    "ipv4_only": false
  }
}
```

**Key design decisions:**
- Config contains NO secrets — only profile metadata
- Tokens are in the OS keychain, referenced by profile name
- `AuthMode` is a typed string enum: `"delegated"` or `"app-only"`
- `Settings.IPv4Only` is a placeholder for the Azure NZ North IPv6 workaround

**Operations:**

| Function | Behaviour |
|----------|-----------|
| `Load()` | Reads config from disk, returns empty config if file doesn't exist |
| `Save()` | Writes config with `MarshalIndent`, creates directory if needed |
| `ActiveProfileConfig()` | Returns the currently active profile or error |
| `SetActiveProfile(name)` | Switches active profile, clears `Active` flag on all others |

### 3.4 Output — `internal/output/`

Three output formats, resolved from flags:

| Format | Flag | Stdout | Stderr | Use Case |
|--------|------|--------|--------|----------|
| Human | (default) | Formatted tables | ✓/✗/→ prefixed messages | Interactive terminal |
| JSON | `--json` | Indented JSON | Messages only | Agent consumption |
| Plain | `--plain` | Tab-separated values | Messages only | Shell scripting |

**Key design decision:** Success/info/error messages always go to stderr. This ensures `cb365 todo tasks list --json | jq .scopes` works without human-readable text contaminating the JSON stream. This is critical for agent consumption.

## 4. Dependencies

| Package | Version | Purpose | Maintainer |
|---------|---------|---------|------------|
| `azure-sdk-for-go/sdk/azidentity` | v1.13.1 | Entra ID authentication | Microsoft |
| `azure-sdk-for-go/sdk/azcore` | v1.21.0 | Token types, HTTP pipeline | Microsoft |
| `AzureAD/microsoft-authentication-library-for-go` | v1.6.0 | MSAL token cache (transitive) | Microsoft |
| `spf13/cobra` | v1.10.2 | CLI command framework | Community (widely adopted) |
| `zalando/go-keyring` | v0.2.8 | OS keychain access | Zalando |
| `golang-jwt/jwt/v5` | v5.3.0 | JWT parsing (transitive via azidentity) | Community |
| `google/uuid` | v1.6.0 | UUID generation (transitive) | Google |
| `godbus/dbus/v5` | v5.2.2 | Linux D-Bus for secret-service (transitive) | Community |
| `danieljoos/wincred` | v1.2.3 | Windows Credential Manager (transitive) | Community |

**Total direct dependencies:** 4 (azidentity, azcore, cobra, go-keyring)
**Total including transitive:** 14

## 5. CI/CD Pipeline

**File:** `.github/workflows/ci.yml`
**Triggers:** Push to `main`, pull requests to `main`

**Jobs:**

| Job | Steps | Purpose |
|-----|-------|---------|
| `build` | checkout → setup-go → build → test → vet | Compilation and correctness |
| `security` | checkout → setup-go → gosec → govulncheck | Security scanning |

**Security tools:**
- **gosec:** Static analysis for Go security issues (SQL injection, path traversal, hardcoded credentials, weak crypto)
- **govulncheck:** Checks dependencies against Go vulnerability database

## 6. File Permissions and Security

| Path | Permissions | Contains Secrets |
|------|-------------|-----------------|
| `~/.config/cb365/` | `0700` | No |
| `~/.config/cb365/config.json` | `0600` | No (profile metadata only) |
| OS Keychain (`cb365` service) | OS-managed | Yes (access tokens) |
| `~/cb365/` (source) | Standard | No |

## 7. Known Limitations

### 7.1 Headless Linux Keychain (CRITICAL — Phase 0 Blocker)

`go-keyring` requires `secret-service` (D-Bus) on Linux, which needs a running keyring daemon (`gnome-keyring-daemon`). On headless servers like `vm-openclaw-01`, this is not available.

**Impact:** `cb365 auth login` will fail on the VM until a fallback is implemented.

**Planned resolution options:**
1. Encrypted file backend with passphrase from `CB365_KEYRING_PASSWORD` env var
2. Linux kernel keyring via `keyctl`
3. `pass` (GPG-based) backend

### 7.2 Azure NZ North IPv6

All HTTPS calls from `vm-openclaw-01` need IPv4 forcing due to broken IPv6 egress. The `Settings.IPv4Only` config field is defined but not yet wired to an `http.Transport` with IPv4-only DNS resolution.

### 7.3 Token Refresh

Phase 0 stores only the access token, not the refresh token. `azidentity`'s device-code credential handles token refresh internally via MSAL's in-memory cache during a single process lifetime, but tokens will expire between invocations. Full refresh token persistence requires MSAL cache serialisation, which is planned for Phase 0.5.

### 7.4 No Graph API Calls

Phase 0 authenticates but makes no Graph API calls. The `msgraph-sdk-go` dependency is not yet added. First Graph calls arrive in Phase 1 (To Do).

## 8. Testing

### 8.1 Current Test Coverage

No unit tests written yet. The CI pipeline includes `go test ./...` but there are no test files. Test authoring is planned alongside Phase 1 development.

### 8.2 Planned Test Strategy

| Layer | Approach |
|-------|----------|
| Auth | Mock `azidentity` credential, verify token flow |
| Config | Temp directory, write/read/verify round-trip |
| Output | Capture stdout/stderr, verify JSON schema |
| Keyring | Mock keyring interface, verify store/load/delete |
| Integration | Microsoft Graph dev tenant, real API calls |

## 9. Deployment

### 9.1 Build

```bash
cd ~/cb365
go build -o cb365 ./cmd/cb365/
```

### 9.2 Install (VM)

```bash
go build -o ~/.local/bin/cb365 ./cmd/cb365/
```

### 9.3 Cross-Compile (Future — Release)

```bash
GOOS=linux GOARCH=amd64 go build -o cb365-linux-amd64 ./cmd/cb365/
GOOS=darwin GOARCH=arm64 go build -o cb365-darwin-arm64 ./cmd/cb365/
GOOS=windows GOARCH=amd64 go build -o cb365-windows-amd64.exe ./cmd/cb365/
```

## 10. Configuration Reference

### 10.1 Environment Variables (Planned)

| Variable | Purpose | Default |
|----------|---------|---------|
| `CB365_KEYRING_PASSWORD` | Passphrase for encrypted file keyring fallback | (none — prompts) |
| `CB365_IPV4_ONLY` | Force IPv4 DNS resolution | `false` |
| `CB365_CONFIG_DIR` | Override config directory | `~/.config/cb365` |

### 10.2 Auth Login Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--tenant` | Yes | — | Entra ID tenant ID or domain |
| `--client` | Yes | — | Entra ID application (client) ID |
| `--scopes` | No | `.default` | Comma-separated Graph scopes |
| `--name` | No | `default` | Profile name |

## 11. Appendix: Dependency Licence Audit

| Package | Licence |
|---------|---------|
| `azure-sdk-for-go` | MIT |
| `microsoft-authentication-library-for-go` | MIT |
| `spf13/cobra` | Apache 2.0 |
| `zalando/go-keyring` | MIT |
| `golang-jwt/jwt` | MIT |
| `google/uuid` | BSD-3-Clause |
| `godbus/dbus` | BSD-2-Clause |
| `danieljoos/wincred` | MIT |

All dependencies are permissive (MIT, Apache 2.0, BSD). No copyleft or restrictive licences.
