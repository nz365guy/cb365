# Design amendment: BWS delegated-cache record v2

> **Item:** [#48](https://github.com/nz365guy/cb365/issues/48)
> **Decision target:** ADR-0057 amendment in Department 10
> **Scope:** Record format and Gate 1 remeasurement only; no provider code,
> tenant operation, credential migration or live BWS mutation is authorised.

## Outcome

Select one BWS secret value with schema `cb365.msal-cache/v2`. The current
MSAL Go cache export is embedded directly as a `json.RawMessage`-style JSON
value instead of a base64 string. The adapter continues to treat the cache as
opaque bytes: it validates JSON syntax but does not inspect, decode, normalise
or reconstruct MSAL fields.

The #42 representative cache remeasurement is **10,643 bytes** for the complete
v2 record against the verified **26,191-byte** BWS plaintext limit. This leaves
**15,548 bytes / 59.36% headroom**, so the unchanged >=50% Gate 1 passes with
2,452 bytes below the 13,095-byte ceiling.

## Record contract

```json
{
  "schemaVersion": "cb365.msal-cache/v2",
  "binding": {
    "tenantId": "<tenant UUID>",
    "clientId": "<client UUID>",
    "homeAccountId": "<MSAL home account ID>",
    "profile": "<profile name>"
  },
  "generation": 1,
  "previousDigest": "<sha256 of previous raw cache bytes, or null>",
  "cache": { "<opaque MSAL JSON value>": "<embedded byte-for-byte>" },
  "updatedAt": "<RFC3339 UTC>",
  "writer": "<non-secret host identity>"
}
```

Normative rules:

1. `cache` is the exact byte slice returned by MSAL `Marshaler.Marshal`, nested
   as one JSON value. It is not base64 encoded and must never be printed.
2. Before writing, validate only that the exported bytes are a complete JSON
   value. A non-JSON export fails closed as `managed_cache_invalid`; it does not
   fall back to v1, another encoding or local storage.
3. After serialising the envelope, extract `cache` again and verify its SHA-256
   equals the pre-serialisation SHA-256. This prevents an encoder from silently
   changing the opaque bytes.
4. `previousDigest` is `sha256:<lower-case hex>` over the exact raw cache bytes
   read at the start of the locked transaction, or `null` for the first write.
   Generation must increase monotonically from the record read under the lock.
5. Post-write readback must verify schema, exact binding, generation,
   `previousDigest`, writer and the SHA-256 of the returned raw `cache` against
   the just-exported bytes before token acquisition is reported successful.
6. v1 is not a migration format. No v1 production record exists because Gate 1
   blocked implementation. A v1 or unknown schema fails closed and requires an
   explicitly reviewed migration path.

MSAL documents the exported representation as opaque and gives callers no
format guarantee. The v2 boundary therefore relies only on the current pinned
MSAL version producing valid JSON; it makes no claim about the JSON schema.
An MSAL upgrade must rerun this measurement and the byte-preservation check
before release.

## Evidence-safe remeasurement

The #42 run retained only sizes and a SHA-256 digest; no cache bytes were
persisted. That is sufficient for an exact v2 remeasurement because the v2
record keeps every metadata field and its serialised length unchanged and
replaces only the cache representation:

```text
v1 complete record                         14,043 bytes
- base64 cache characters                -13,592 bytes
- JSON string quotes around base64 value      -2 bytes
+ exact raw cache JSON                    +10,194 bytes
                                         -------------
v2 complete record                         10,643 bytes

BWS plaintext limit                        26,191 bytes
remaining headroom                         15,548 bytes
headroom             15,548 / 26,191 =      59.36%
50% ceiling                                  13,095 bytes
margin below ceiling                           2,452 bytes
Gate 1                                             PASS
```

The non-secret provenance digest remains
`eb14f1383ccd9660ca351dd285f0133cd1f7973384f3696bad260c0ad5284288`.
The #42 tool is amended to construct v2 directly on any future authorised run
and report only sizes, this digest, headroom and PASS/FAIL.

## Components and data flow

1. The profile-scoped volatile lock is acquired on its assigned host.
2. The adapter reads the one bound BWS record and rejects unknown schema,
   binding mismatch, malformed JSON or a stale generation.
3. The raw `cache` value is passed unchanged to MSAL `Unmarshaler.Unmarshal`.
4. Login or silent acquisition runs while the lock remains held.
5. MSAL exports bytes; the adapter validates JSON syntax and computes their
   SHA-256 in memory.
6. The v2 envelope is serialised, its embedded cache digest is checked, and the
   one BWS record is updated with the next generation and prior digest.
7. The adapter reads the record back and verifies binding, generation and the
   exported-cache digest before releasing the lock or allowing a workload
   request.

## Trade-offs

| Option | Decision | Reason |
| --- | --- | --- |
| Direct opaque JSON value in one v2 record | Selected | Removes 2,951 bytes from the measured v1 record and passes Gate 1 while preserving one-record atomicity. |
| Keep base64 v1 | Rejected | Measured at 46.38% headroom, below the mandatory gate. |
| Lower the 50% margin | Rejected | Removes growth protection without fixing the encoding overhead. |
| Split or truncate the cache | Rejected | Breaks the one-record contract or MSAL's complete opaque-cache contract. |
| Compress the cache | Rejected | Adds format and decompression failure modes and is outside #48. |
| Restore local fallback | Rejected | Violates the BWS-only credential boundary and fail-closed invariant. |

The selected format assumes only that the pinned MSAL export is valid JSON.
That assumption is narrower than understanding its fields, but it must be
revalidated on every MSAL dependency upgrade.

## Dependencies and downstream scope

- Gate 2 from #42 carries forward unchanged: Bitwarden Go SDK v2.1.0 and both
  cgo/no-cgo build paths passed.
- #43 must implement v2 only, enforce byte-preservation and unknown-schema
  failure, and retain all ADR-0057 binding, locking, readback, lifecycle and
  no-local-fallback requirements.
- #44 and `cloverbase-dev-team#460` remain blocked behind the accepted ADR
  amendment and delivered provider; this design does not relax their gates.
- No new package, service or SaaS spend is introduced by this amendment.

## Security and monitoring

The cache remains bearer material even though it is syntactically JSON. It may
exist only in process memory and the one BWS EU secret value. Cache bytes must
not enter stdout, stderr, logs, argv, environment, temporary files, panic
values or committed artefacts. Digests and sizes are evidence-only and are not
routine telemetry.

Production monitoring remains secret-free: profile, host, duration,
generation and stable outcome class only. Alert on repeated
`managed_cache_invalid`, `managed_cache_conflict`, readback failure,
unavailability or reauthentication. Do not log record sizes or digests during
normal operation.

## Primary references

- [MSAL Go external cache contract](https://pkg.go.dev/github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache)
- [Go `json.RawMessage`](https://pkg.go.dev/encoding/json#RawMessage)
- [ADR-0057](https://github.com/nz365guy/cloverbase-dept-10-technology/blob/main/docs/adr/ADR-0057-bws-backed-delegated-msal-cache.md)
- [#42 spike evidence](./SPIKE.md)
