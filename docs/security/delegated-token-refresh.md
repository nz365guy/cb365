# Security Design: Delegated Token Auto-Refresh Lifecycle

> **Item:** [#37](https://github.com/nz365guy/cb365/issues/37) (split from [#20](https://github.com/nz365guy/cb365/issues/20) per Mark's Option A decision)
> **Work type:** sec · **Status:** Design for review · **Date:** 2026-07-20
> **Scope:** `internal/auth/` — delegated device-code authentication, credential storage, token lifecycle.
> **This document authorises no implementation.** Gaps found here become separate, explicitly approved items.

---

## 1. Current State (What Is Already Built)

Silent delegated refresh **already exists on `main`** — it shipped in
`4e30eec feat(auth): MSAL persistent cache for delegated token auto-refresh`,
with CAE alignment (`099c407`) and a macOS no-cgo fallback (`9068d62`, PR #31).
This design documents that implementation, defines its required lifecycle
controls, and identifies the gaps.

The refresh path:

1. `cb365 auth login` (delegated) → `LoginDelegatedWithCache` runs device-code flow with the MSAL persistent cache and returns an `AuthenticationRecord`.
2. The `AuthenticationRecord` (JSON, **not a secret** — account/tenant metadata for cache lookup) is stored in the cb365 `TokenCache`.
3. On expiry, `GetTokenSilent` → `NewDelegatedCredentialSilent` rebuilds the credential from the stored record and redeems the **refresh token held inside the MSAL cache**. cb365 code never touches the refresh token directly.
4. In unattended contexts the credential's `UserPrompt` returns an error, so a refresh that would need interaction **fails fast** instead of printing a device code nobody will see.

### Two storage planes

| Plane | Contents | Backend |
|-------|----------|---------|
| cb365 `TokenCache` | Access token, `AuthenticationRecord`, expiry metadata | OS keychain via `go-keyring` (Windows Credential Manager / macOS Keychain / Linux secret-service); fallback: AES-256-GCM encrypted file `~/.config/cb365/tokens.enc` |
| MSAL persistent cache (`azidentity/cache`, name `cb365`) | Access **and refresh tokens** | Windows: DPAPI · Linux: kernel keyring · macOS: **in-memory only** (persistent backend needs cgo; release builds disable it) |

Encrypted-file fallback: PBKDF2-SHA256, 210,000 iterations (OWASP 2023), AES-256-GCM,
key derived from `CB365_KEYRING_PASSWORD`, atomic writes, `0600`/`0700` permissions.

---

## 2. Approved Storage Pattern (AC 1)

**Approved:** the existing two-plane pattern above. **No plaintext token file exists in any path.**

Rules:

- **R1 — Passphrase sourcing.** On headless hosts (the openclaw VM), `CB365_KEYRING_PASSWORD` MUST be injected from **BWS EU** at runtime (the ADR-0042 pattern: BWS → tmpfs-backed env, never a plaintext `.env` or `.bashrc` on disk).
- **R2 — No new services.** No paid secret-storage service is introduced; BWS EU is the already-approved manager.
- **R3 — macOS limitation is accepted.** Release builds do not persist refresh tokens on macOS; silent refresh works only within a process lifetime there. Interactive re-login is the fallback. Revisit only if a macOS unattended use case appears.
- **R4 — File permissions.** Encrypted-file store creates the directory `0700` and writes the file `0600` (re-applied on every save). Permissions are **not verified on load** — fail-closed checking is gap **G4** below.

---

## 3. Credential Lifecycle (AC 2)

| Event | Behaviour | Where |
|-------|-----------|-------|
| **Rotation** | Refresh tokens are rotated by Entra ID on each redemption; MSAL handles this. cb365 never implements its own rotation and never reads the raw refresh token. | MSAL / `azidentity` |
| **Access-token expiry** | ~60–90 min lifetime. `auth status` (and workload commands via silent credential) detect expiry and renew silently from the MSAL cache. | `cmd/cb365/auth.go` |
| **Refresh-token expiry** | Sliding ~90-day inactivity window (tenant policy). `auth status` warns below `CB365_REFRESH_WARN_DAYS` (default 14) using an issued-at heuristic — documented as **approximate**, not authoritative. | `cmd/cb365/auth.go` |
| **Revocation** | Admin revokes sessions in Entra → next refresh redemption fails. `GetTokenSilent` returns a clear "re-run `cb365 auth login`" error, non-zero exit, no retry loop. CAE (`EnableCAE: true`) additionally lets Entra invalidate live access tokens via claims challenge. | `internal/auth/credential.go` |
| **Missing credential** | No stored `AuthenticationRecord` or empty cache → fail fast with an actionable error; never starts an interactive device-code prompt in an unattended context. | `internal/auth/credential.go` |
| **Logout** | `auth logout` deletes the `TokenCache` entry and profile. **GAP G1: it does not clear the MSAL persistent cache**, so a valid refresh token survives logout until expiry/revocation. | `cmd/cb365/auth.go` |

---

## 4. Redaction and Exposure Controls (AC 3)

| Control | State |
|---------|-------|
| `TokenCache` marked "never log or print" | ✅ In place (`keyring.go`) |
| Token display goes through `DecodeTokenInfo` (claims only, never the raw JWT) | ✅ In place |
| `--verbose` prints refresh *status* only ("Token expired — refreshing…"), never token material | ✅ In place |
| Errors wrap causes without embedding tokens | ✅ In place |
| Delegated flow takes **no secret via flag or stdin** — device-code only, so no shell-history exposure exists for this flow | ✅ By design |
| `CB365_KEYRING_PASSWORD` must come from BWS EU injection, not typed inline (inline `export` lands in shell history) | ⚠️ Operational rule R1 — verify on the VM |
| Azure SDK debug logging (`AZURE_SDK_GO_LOGGING`) must remain unset in agent environments; it can log HTTP traffic | ⚠️ **GAP G2** — add to ops checklist / env hardening |
| CI: `gosec` (gating) on push/PR to `main`; CodeQL weekly | ✅ In place |
| CI: `govulncheck` — runs but is `continue-on-error` pending Go 1.25.9 stdlib fixes (GO-2026-4601/4870/4946), so it does not currently gate | ⚠️ Gap **G5** |

---

## 5. Verification Test Design (AC 4)

All tests run in the **test tenant** with a dedicated test profile. Evidence = command
transcripts with timestamps, recorded on #37. Expected grep target for leakage checks:
JWT prefix `eyJ` outside approved stores.

| # | Test | Steps | Pass criteria |
|---|------|-------|---------------|
| T1 | **Silent renewal after expiry** | Delegated login → wait out access-token lifetime (or shorten via tenant token-lifetime policy) → `cb365 auth status --verbose` | "refreshing via MSAL cache" then success; **zero interaction**; new expiry in output |
| T2 | **Safe failure after revocation** | Revoke sessions (Entra portal or `Revoke-MgUserSignInSession`) → allow propagation → `cb365 auth status` | Non-zero exit; error instructs re-login; no token material in output |
| T3 | **Missing credential** | `cb365 auth logout` (or delete keyring entry) → any workload command | Clear "run `cb365 auth login`" error; no interactive prompt hang in unattended mode |
| T4 | **No leakage** | After T1–T3: grep shell history, cb365 output logs, `~/.config/cb365/` and process environment for `eyJ` | No token outside OS keychain / kernel keyring / `tokens.enc` |

T1/T2 are manual with recorded evidence (they need a live tenant); T3/T4 are
scriptable and belong in `test/integration/` once an implementation item is approved.

---

## 6. Decisions Required / Recorded (AC 5)

- **ADR: yes.** The credential-storage direction (two-plane MSAL cache + keyring/encrypted-file, BWS EU passphrase sourcing, macOS in-memory limitation) should be recorded as an ADR in `cloverbase-dept-10-technology` so it binds future work.
- **Separate implementation items: yes.** This document authorises none of them:
  - **G1** — clear the MSAL persistent cache entry on `auth logout` (refresh token must not survive logout).
  - **G2** — environment hardening: assert `AZURE_SDK_GO_LOGGING` unset in agent/VM profiles; add to ops checklist.
  - **G3** — automate T3/T4 in `test/integration/`; record T1/T2 evidence on #37.
  - **G4** — verify token-store file/directory permissions on load and fail closed on unexpected modes (today they are only set at creation/save).
  - **G5** — restore `govulncheck` as a gating CI check once setup-go ships Go >= 1.25.9 (tracked by the TODO in `.github/workflows/ci.yml`).

## 7. Explicitly Out of Scope

- Any change to app-only authentication (unchanged by design).
- Teams `--html` rendering (delivered separately under #20).
- New storage services or plaintext token files (prohibited).
