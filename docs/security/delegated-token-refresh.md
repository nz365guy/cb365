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
controls, and identifies the gaps. The shipped storage is observed current
state, not the approved target: delegated access and refresh material currently
lives outside BWS EU and therefore does not satisfy the BWS-only secret rule.

The refresh path:

1. `cb365 auth login` (delegated) → `LoginDelegatedWithCache` runs device-code flow with the MSAL persistent cache and returns an `AuthenticationRecord`.
2. The `AuthenticationRecord` (JSON, **not a secret** — account/tenant metadata for cache lookup) is stored in the cb365 `TokenCache`.
3. On expiry, `GetTokenSilent` → `NewDelegatedCredentialSilent` rebuilds the credential from the stored record and redeems the **refresh token held inside the MSAL cache**. cb365 code never touches the refresh token directly.
4. In unattended contexts the credential's `UserPrompt` returns an error, so a refresh that would need interaction **fails fast** instead of printing a device code nobody will see.

### Current storage planes

| Plane | Contents | Backend |
|-------|----------|---------|
| cb365 `TokenCache` | Access token, `AuthenticationRecord`, expiry metadata | OS keychain via `go-keyring` (Windows Credential Manager / macOS Keychain / Linux secret-service); fallback: AES-256-GCM encrypted file `~/.config/cb365/tokens.enc` |
| MSAL persistent cache (`azidentity/cache`, name `cb365`) | Access **and refresh tokens** | Windows: DPAPI · Linux: kernel keyring · macOS: **in-memory only** (persistent backend needs cgo; release builds disable it) |

Encrypted-file fallback: PBKDF2-SHA256, 210,000 iterations (OWASP 2023), AES-256-GCM,
key derived from `CB365_KEYRING_PASSWORD`, atomic writes, `0600`/`0700` permissions.

Both rows contain bearer credentials. OS keychains and the encrypted-file
fallback reduce plaintext exposure, but they are not BWS EU and are therefore
transitional for delegated profiles.

---

## 2. Approved Storage Pattern (AC 1)

**Approved target:** all delegated access-token and refresh/cache material is
stored in **Bitwarden Secrets Manager EU**. No delegated bearer credential may
fall back to the OS keychain, Azure Identity's local persistent cache, the
encrypted-file store, a plaintext file or an environment variable.

The supported design is an MSAL Go public client configured with
`public.WithCache(cache.ExportReplace)`, backed by a BWS EU adapter. MSAL treats
the exported cache as opaque and recommends one MSAL instance per user; cb365
therefore uses one versioned BWS record per tenant, client ID,
home-account/profile binding. Azure Identity's public `azidentity.Cache` cannot
accept an external store, so copying private cache internals or scraping local
cache files is prohibited.

Rules:

- **R1 — BWS-only delegated cache.** The opaque MSAL cache and any delegated
  `TokenCache` bearer-token fields are stored in BWS EU. Non-secret profile and
  authentication-record metadata may remain local only when it contains no
  bearer credential.
- **R2 — Least-privilege machine identity.** The BWS machine account has
  read/write access only to the dedicated cb365 cache project or records. Its
  bootstrap credential arrives through the existing approved runtime injection
  path and is never placed in source, profile configuration, a command-line
  argument, shell history or logs.
- **R3 — No BWS client residue.** Prefer a supported in-process SDK. If the BWS
  CLI is used, its authentication state is opted out or confined to an approved
  volatile path; default persistent state is not accepted.
- **R4 — Fail closed.** BWS unavailability, malformed cache data, a binding
  mismatch or an export failure produces a non-zero error and no interactive
  fallback, local-cache fallback or workload request.
- **R5 — One writer per profile.** Until the chosen BWS interface supplies a
  proven compare-and-swap primitive, a delegated profile is assigned to one
  host and guarded by a per-profile process lock. Cross-host concurrent refresh
  is unsupported. Every write includes a version, checks the prior revision and
  is read back before success is reported.
- **R6 — Migration is fail closed.** A migration copies and validates cache
  state in BWS before deleting and verifying all legacy local delegated cache
  layers. Partial migration is visible and cannot enable workload requests.
- **R7 — No new services.** No paid secret-storage service is introduced; BWS
  EU is the already-approved manager.
- **R8 — App-only remains separate.** This item changes no app-only credential
  behaviour or permission.

### Bound record format selected by #48

The Gate 1 spike found that ADR-0057's base64 `cb365.msal-cache/v1` envelope
left only 46.38% headroom. The selected amendment is one
`cb365.msal-cache/v2` BWS record with the current MSAL JSON export embedded
directly as a `json.RawMessage`-style value. This removes base64 inflation
without splitting, truncating, compressing or interpreting the opaque cache.

The v2 envelope retains the exact tenant/client/home-account/profile binding,
monotonic generation, prior-cache digest, timestamp and writer. Serialisation
must preserve the embedded cache bytes exactly. The post-write BWS readback
verifies the binding, generation and SHA-256 of the embedded cache against the
just-exported bytes before any workload request begins. Unknown schemas and a
future MSAL export that is not valid JSON fail closed; they do not trigger a
v1 or local-storage fallback.

The evidence-safe #42 remeasurement is 10,643 bytes for the complete v2 record
against the 26,191-byte BWS plaintext limit: 15,548 bytes / 59.36% headroom,
passing the unchanged >=50.00% gate. The format, exact arithmetic, trade-offs
and upgrade guard are specified in
[`docs/plan/37-bws-delegated-cache/DESIGN-AMENDMENT.md`](../plan/37-bws-delegated-cache/DESIGN-AMENDMENT.md).
Provider implementation remains blocked until the Department 10 ADR-0057
amendment is accepted.

---

## 3. Credential Lifecycle (AC 2)

| Event | Behaviour | Where |
|-------|-----------|-------|
| **Rotation** | Entra can return replacement refresh material during redemption. MSAL handles the opaque cache update; the BWS `ExportReplace` adapter must persist and read back the new cache before cb365 reports success. Older server-side tokens are not assumed revoked. | MSAL + BWS adapter |
| **Access-token expiry** | ~60–90 min lifetime. `auth status` (and workload commands via silent credential) detect expiry and renew silently from the BWS-backed MSAL cache. | `cmd/cb365/auth.go` + BWS adapter |
| **Refresh-token expiry** | The Microsoft identity platform default is 90 days for this non-SPA flow, subject to tenant policy and earlier revocation. Redemption can return replacement refresh material without revoking the older token. `auth status` warns below `CB365_REFRESH_WARN_DAYS` (default 14) using an issued-at heuristic — documented as **approximate**, not authoritative. | `cmd/cb365/auth.go` |
| **Revocation** | Admin revokes sessions in Entra → next refresh redemption fails. `GetTokenSilent` returns a clear "re-run `cb365 auth login`" error, non-zero exit, no retry loop. CAE (`EnableCAE: true`) additionally lets Entra invalidate live access tokens via claims challenge. | `internal/auth/credential.go` |
| **Missing credential / BWS unavailable** | No bound BWS record, an empty cache or an unavailable BWS service fails fast with an actionable error; unattended commands never start device code and never fall back locally. | BWS adapter + `internal/auth/credential.go` |
| **Concurrent refresh** | One process/host holds the profile lock; a stale revision fails rather than overwriting newer cache state. | BWS adapter |
| **Logout** | Target behaviour deletes and verifies the BWS cache record, local authentication record and every legacy delegated cache layer, then reports that Entra revocation is a separate operator action. **Current gap:** `auth logout` deletes only the cb365 `TokenCache` and profile, so the shipped MSAL refresh token survives. | `cmd/cb365/auth.go` + BWS adapter |

---

## 4. Redaction and Exposure Controls (AC 3)

| Control | State |
|---------|-------|
| `TokenCache` marked "never log or print" | ✅ In place (`keyring.go`) |
| Token display goes through `DecodeTokenInfo` (claims only, never the raw JWT) | ✅ In place |
| `--verbose` prints refresh *status* only ("Token expired — refreshing…"), never token material | ✅ In place |
| Errors wrap causes without embedding tokens | ✅ In place |
| Delegated flow takes **no secret via flag or stdin** — device-code only, so no shell-history exposure exists for this flow | ✅ By design |
| Delegated bearer credentials never use `CB365_KEYRING_PASSWORD`, OS keychain or encrypted-file fallback | ⚠️ **GAP G1** — BWS-backed MSAL migration required |
| BWS machine credential and client state are runtime-only and fully redacted | ⚠️ **GAP G1** — implement and verify with the adapter |
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
| T1 | **Silent renewal after expiry** | BWS-backed delegated login → wait out access-token lifetime → `cb365 auth status --verbose` | BWS-backed refresh succeeds with zero interaction; new expiry; record revision advances |
| T2 | **Safe failure after revocation** | Revoke sessions in Entra → allow propagation → `cb365 auth status` | Non-zero exit; error instructs re-login/revocation handling; no retry loop or token material |
| T3 | **Missing/unavailable BWS** | Remove access to the bound record and separately make BWS unavailable → any unattended workload command | Clear non-zero error; zero Entra/workload requests; no device prompt or local fallback |
| T4 | **No leakage or residue** | After T1–T3, inspect history, output, logs, process environment, BWS client state and legacy cb365/MSAL stores | No delegated bearer token or cache outside the bound BWS EU record; no unmanaged BWS state |
| T5 | **Concurrent refresh** | Start two refreshes for one profile and attempt a stale revision write | One writer succeeds; the stale writer fails without overwriting the newer BWS record |
| T6 | **Migration and logout** | Migrate a legacy test profile, then log out | BWS read-back succeeds before legacy deletion; logout verifies BWS and every legacy cache layer absent |

T1/T2 require a test-tenant operator and recorded evidence. T3–T6 are
automatable and belong in `test/integration/` under a separately approved
implementation item.

---

## 6. Decisions Required / Recorded (AC 5)

- **ADR: yes.** The BWS EU-backed MSAL external-cache direction, identity
  binding, one-writer constraint, migration, revocation and no-fallback rule
  should be recorded in `cloverbase-dept-10-technology` before implementation.
- **Separate implementation items: yes.** This document authorises none of them:
  - **G1** — implement the BWS EU `cache.ExportReplace` adapter, migrate
    delegated profiles, remove delegated local-cache fallback and make logout
    clear and verify BWS plus legacy cache state. Implement the accepted
    `cb365.msal-cache/v2` single-record contract only; reject v1 and unknown
    schemas fail closed.
  - **G2** — environment hardening: assert `AZURE_SDK_GO_LOGGING` unset in agent/VM profiles; add to ops checklist.
  - **G3** — automate T3–T6 in `test/integration/`; record T1/T2 evidence on #37.
  - **G4** — verify legacy token-store file/directory permissions during migration and fail closed on unexpected modes before reading them.
  - **G5** — restore `govulncheck` as a gating CI check once setup-go ships Go >= 1.25.9 (tracked by the TODO in `.github/workflows/ci.yml`).

## 7. Explicitly Out of Scope

- Any change to app-only authentication (unchanged by design).
- Teams `--html` rendering (delivered separately under #20).
- New storage services or plaintext token files (prohibited).

## 8. Primary References

- [MSAL Go externally managed cache contract](https://pkg.go.dev/github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache)
- [MSAL Go public-client `WithCache`](https://pkg.go.dev/github.com/AzureAD/microsoft-authentication-library-for-go/apps/public#WithCache)
- [Microsoft identity platform refresh tokens](https://learn.microsoft.com/en-us/entra/identity-platform/refresh-tokens)
- [Bitwarden Secrets Manager CLI](https://bitwarden.com/help/secrets-manager-cli/)
- [Bitwarden Secrets Manager machine accounts and access tokens](https://bitwarden.com/help/secrets-manager-quick-start/)
