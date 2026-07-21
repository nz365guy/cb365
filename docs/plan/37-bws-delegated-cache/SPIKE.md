# SPIKE: ADR-0057 pre-implementation gates (cb365 #42)

Proves (or refutes) the two mandatory assumptions of
[ADR-0057](https://github.com/nz365guy/cloverbase-dept-10-technology/blob/main/docs/adr/ADR-0057-bws-backed-delegated-msal-cache.md)
before any credential code is written for the BWS EU–backed delegated MSAL
cache. Design container: [cb365 #37](https://github.com/nz365guy/cb365/issues/37).
FAIL on either gate routes the work back to Design per ADR-0057 — the cache
must not be split, truncated or compressed to force a fit.

All evidence in this directory is size/digest/version only. The JWT base64url
prefix scan over these artefacts is enforced by
`.github/workflows/spike-37.yml` and by `spike/build.sh`.

---

## Gate 1 — BWS secret-value headroom

**Requirement:** a representative exported MSAL cache (test-tenant,
test-profile, operator-assisted device-code login) must fit the verified BWS
secret-value limit with ≥ 50% headroom, measured on the full ADR-0057
envelope (base64 cache + binding metadata), since the envelope is what is
stored.

### Verified limit

- **Server-enforced bound (pinned source):** Bitwarden server validates the
  secret `Value` with `[EncryptedStringLength(35000)]` —
  [SecretCreateRequestModel.cs @ fa0c9cb](https://github.com/bitwarden/server/blob/fa0c9cb38785a867cc3b12e5678051004ba6362b/src/Api/SecretsManager/Models/Request/SecretCreateRequestModel.cs).
  The same bound applies to updates (`SecretUpdateRequestModel`). Note this
  limits the **EncString ciphertext representation**, not the plaintext; the
  effective plaintext limit is smaller (ciphertext is base64 + IV/MAC
  overhead, ≈ 3/4 × 35000 minus fixed overhead ≈ ~26 KB).
- **Effective plaintext limit (analytic, from the pinned bound):** SM secret
  values are encrypted as EncString type 2 (`AesCbc256_HmacSha256_B64`,
  format `2.<b64 iv>|<b64 ct>|<b64 mac>`): fixed overhead 72 chars
  (2 + 24 iv + 2 separators + 44 mac), ciphertext budget 34,928 base64 chars
  → 26,196 ciphertext bytes → largest AES block multiple 26,192 → minus
  mandatory PKCS7 padding byte = **26,191 plaintext bytes**.
  ≥ 50% headroom therefore requires the envelope ≤ **13,095 bytes**.
- **Empirical verification:** not run in this spike — no write-capable BWS
  machine token exists outside root on the VM (operator decision 2026-07-21:
  verify analytically now). `spike/limit-probe.sh` is committed for reuse.
  Residual risk is closed by ADR-0057 itself: the first live envelope write
  in cb365 #43 must be read back and digest-verified before any use, which
  empirically confirms the limit before production rollout.

### Measurement

Tool: `spike/cmd/measure` — MSAL Go public client (`public.WithCache`) with an
in-memory `cache.ExportReplace` capture: one device-code login by the
operator (Mark), then one silent acquisition (the steady-state refresh
path). Cache bytes never touch disk, argv or output.

> **Recorded deviation (operator/owner decision, Mark, 2026-07-21):** no
> dedicated test tenant exists; `cloverbase.com` resolves to the production
> tenant. Mark explicitly waived the test-tenant boundary for this single
> measurement login with his own account, on the basis that the tool holds
> the cache in memory only and records sizes and digests exclusively. The
> waiver applies to this measurement only; T1/T2 tenant evidence in #44
> still requires the test-tenant procedure or a fresh recorded decision.
> Scopes used: `.default` (matches cb365's `work-delegated` profile
> behaviour, so the measured cache is representative of real usage).

Run 2026-07-21 (NZT) on the OpenClaw VM by Mark (operator): device-code
login, `.default` scopes, one export observed (the post-login cache write;
the subsequent silent acquisition was served from the in-memory cache and
correctly triggered no further export).

| Metric                                       | Value     |
| -------------------------------------------- | --------- |
| Raw exported cache (bytes)                    | 10,194    |
| Raw cache SHA-256                             | `eb14f1383ccd9660ca351dd285f0133cd1f7973384f3696bad260c0ad5284288` |
| Base64 cache (chars)                          | 13,592    |
| **ADR-0057 envelope (bytes)**                 | **14,043** |
| Verified plaintext limit (bytes)              | 26,191 (analytic, above) |
| 50%-headroom ceiling (bytes)                  | 13,095    |
| **Headroom** = (limit − envelope) / limit     | **46.4%** |

### Verdict

**Gate 1: FAIL** (2026-07-21). The representative envelope (14,043 bytes)
fits the verified limit but exceeds the ≥50% headroom ceiling of 13,095
bytes by 948 bytes — headroom is 46.4%. Per ADR-0057 and issue #42 this
routes the delegated-cache work back to **Design**: the cache must not be
silently split, truncated or compressed to force a fit. Notes for Design:

- The base64 encoding of the opaque cache is the dominant cost
  (+3,398 bytes, 33% over raw); envelope metadata is only 451 bytes.
- The measurement used a production account with full `.default` consented
  scopes — deliberately representative of real usage rather than a minimal
  test-tenant token; slimmer scope sets would measure smaller but would not
  reflect the deployed profile.
- The gap is small (envelope is 53.6% of the limit vs the required ≤50%),
  so plausible Design responses include revisiting the headroom margin with
  measured growth data, an envelope format without base64 inflation, or a
  documented multi-record contract — all of which are ADR-level decisions,
  not spike scope.

---

## Gate 2 — Bitwarden Go SDK v2 cgo build feasibility

**Requirement:** the official Bitwarden Go SDK v2 native artefact pinned by
version and checksum, licence and provenance verified, `CGO_ENABLED=1` build
of a minimal adapter stub succeeding on the Linux OpenClaw VM toolchain, and
a `CGO_ENABLED=0` build compiling the fail-closed unavailable-provider stub.

### Pinned dependency manifest

| Item | Value |
| ---- | ----- |
| Module | `github.com/bitwarden/sdk-go/v2` |
| Version | `v2.1.0` (latest release, 2026-05-22) |
| Module hash (go.sum) | `h1:DtgklUXNA3GcP5t1eXEEefd0UY6Gv5041/+gZHD2174=` |
| go.mod hash (go.sum) | `h1:6Sfb4IdZ9tnggeFj8Ty4MLkWUyC2pNlFUoAZE0Dapfw=` |
| Native artefact | `internal/cinterface/lib/linux-x64/libbitwarden_c.a`, **vendored inside the Go module** (statically linked; covered by the module hash above and verified by `go mod verify` against the Go checksum database) |
| `libbitwarden_c.a` SHA-256 (linux-x64) | `bbdc2dc8c59659bc8cfc7e4a1d86bcae3f4b9be739411ff25056f99c9fa4ef31` |
| Licence | Bitwarden Software Development Kit License Agreement, Version 1 (2023-03-17) — permits use for developing applications that interoperate with Bitwarden services, which is exactly this use. **Not an OSI licence**; noted for SBOM/licence review in the implementation item. |
| Provenance | Fetched via `proxy.golang.org`, validated against `sum.golang.org` (`go mod verify`); upstream source `github.com/bitwarden/sdk-go` (mirrors `bitwarden/sdk-sm` languages/go). |
| MSAL (measurement + adapter interface) | `github.com/AzureAD/microsoft-authentication-library-for-go v1.7.2` (matches cb365's existing indirect pin) |

Supported statically linked targets per the SDK's INSTRUCTIONS.md: Linux
x86-64/arm64, macOS x86-64/arm64, Windows x86-64 — the initial production
target (Linux OpenClaw VM, x86-64) is covered.

### Build evidence

- Toolchain: VM base Go 1.24.13 with `GOTOOLCHAIN=auto` — go.mod's
  `go 1.25.0` auto-downloads and uses `go1.25.0` (verified; this refutes the
  "VM lacks Go 1.25" blocker recorded on #42), GCC 13.3 (glibc).
- `spike/build.sh` output: `spike/evidence/build-linux-openclaw-vm.txt`
  (VM run) — both variants build; the cgo probe constructs and closes a real
  SDK client (FFI init, no network); the no-cgo probe returns the exact
  ADR-0057 stub error `managed delegated authentication unavailable on this
  build`.
- CI: `.github/workflows/spike-37.yml` repeats both variants on
  ubuntu-latest and enforces the sentinel scan on every PR touching this
  spike.

### Verdict

**Gate 2: PASS** (2026-07-21, VM evidence run above). The pinned SDK v2.1.0
builds and statically links on the Linux OpenClaw VM toolchain with plain
glibc GCC 13.3 (musl not required); the FFI client constructs and closes
cleanly; the `CGO_ENABLED=0` stub compiles and fails closed with the exact
ADR-0057 error string. Checksums are pinned via go.sum and verified against
the Go checksum database; the native artefact digest is recorded above.

---

## Handoff

**Outcome: Gate 1 FAIL / Gate 2 PASS → back to Design per ADR-0057.**

- The G1 provider implementation ([cb365 #43](https://github.com/nz365guy/cb365/issues/43))
  must not start until a superseding Design decision resolves the headroom
  shortfall.
- Gate 2's evidence (pinned SDK, both build variants, licence/provenance)
  remains valid and carries forward to whatever Design decides.
- Failing numbers commented on #37 and #42.
