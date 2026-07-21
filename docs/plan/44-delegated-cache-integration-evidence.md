# Integration evidence contract: managed delegated cache

> **Item:** [#44](https://github.com/nz365guy/cb365/issues/44)
> **Depends on:** [#43](https://github.com/nz365guy/cb365/issues/43) delivering the
> `cb365.msal-cache/v2` provider
> **Status:** Plan complete; implementation and tenant execution pending #43
> **Scope:** T3–T6 integration automation and bounded T1/T2 test-tenant evidence

## Purpose and boundaries

This is the executable-evidence contract for the managed delegated-cache
provider designed in [#37](https://github.com/nz365guy/cb365/issues/37) and
the v2 record amendment in
[`DESIGN-AMENDMENT.md`](37-bws-delegated-cache/DESIGN-AMENDMENT.md). It defines
what the integration suite and operator evidence must prove; it does not
authorise provider implementation, a BWS mutation, Entra consent, session
revocation, a production-profile action, or a tenant operation.

All automated cases use a fake BWS transport, a disposable test profile, and
an opaque token-shaped sentinel. The sentinel must be constructed at runtime,
not checked into source as a JWT prefix. Tests must not make any Entra or
workload HTTP request. T1 and T2 are the only live-tenant cases and must use
the approved test profile only.

## Test-harness contract

The #43 provider must expose test seams for a fake BWS client, a workload HTTP
transport that counts requests, a non-interactive prompt implementation, a
profile-scoped lock, clock control and isolated legacy-store paths. Test setup
must direct process home/config/cache and BWS client-state paths to a per-test
temporary directory. No test may inherit an operator home directory, BWS state
or credential environment.

For each case, capture only these secret-free artefacts:

- exit status and stable error class;
- redacted stdout and stderr;
- structured log events with token/cache fields omitted;
- request counters for Entra and workload transports;
- command argv, relevant environment variable *names* and temporary paths;
- BWS fake operation history containing record identifiers and generations,
  never values.

At teardown, scan captured artefacts and the isolated temporary directory for
the runtime sentinel. The scan must include stdout, stderr, logs, command
arguments, environment values, fake-BWS state, legacy-store paths and any BWS
client-state path. A match fails the test. The required JWT-prefix detector is
represented in source as concatenated literals (`"e" + "yJ"`) so the detector
itself cannot become a repository artefact.

## Automated cases (T3–T6)

| ID | Fixture and action | Required assertions | Verification |
| --- | --- | --- | --- |
| T3a | Bound v2 record absent; invoke one unattended workload command. | Non-zero exit with the documented missing-record error class; Entra requests = 0; workload requests = 0; prompt calls = 0; legacy-store reads = 0. | Integration test plus artefact scan. |
| T3b | Fake BWS read returns unavailable/denied; invoke the same command. | Non-zero exit with the documented unavailable error class; Entra/workload requests = 0; prompt calls = 0; no legacy fallback. | Integration test plus artefact scan. |
| T4 | Execute T3a, T3b, T5 and T6 with the opaque sentinel present only in the fake bound record. | All captured artefacts and isolated residue paths are sentinel-free; no BWS client state remains outside the declared temporary path. | Teardown scanner and path inventory. |
| T5 | Seed one valid bound record; run two refresh transactions behind a deterministic lock barrier so both begin from the same generation. | Exactly one write/readback succeeds; the other returns the stale-generation/conflict error class; final record generation is incremented once and contains the successful writer's digest. | Integration test with fake BWS operation history. |
| T6a | Seed legacy test stores with valid, correctly permissioned fixture data; run explicit single-profile migration. | Provider verifies legacy ownership/modes before read; BWS write then readback/binding/digest verification occur before deletion; workload remains blocked until migration completes; every legacy layer is absent after success. | Integration test with ordered fake-client history and filesystem inspection. |
| T6b | Repeat T6a with an invalid legacy file or directory owner/mode, then with BWS readback failure. | Migration fails closed before an unsafe read or before deletion respectively; no workload request; source legacy layer remains when readback fails. | Integration test plus filesystem inspection. |
| T6c | After a migrated/login fixture, run `auth logout`. | Bound BWS record deletion is verified; delegated local fields and all legacy cache layers are absent; output says Entra session revocation is a separate operator action; no token/cache value is emitted. | Integration test plus artefact scan. |

Stable error-class names and the precise command entry point are implementation
details owned by #43, but must be asserted as typed/sentinel errors rather than
unstable full error strings. Any error must be actionable and must not embed a
record value, cache bytes, machine-access token or delegated bearer material.

## CI requirements

Place the automated suite under `test/integration/` and run it in the supported
cgo-enabled build. Run a compile-and-fail-closed check for `CGO_ENABLED=0` as
defined by ADR-0057. The CI job must use no BWS credential and must fail if a
credential-like environment variable is supplied. Its artefacts may retain
test name, timestamp, outcome class, request counts and redacted path labels;
they must not upload raw temporary directories, environment dumps, logs with
unfiltered values, BWS responses or cache files.

Required gates:

1. T3–T6 pass with fake transports and all zero-request assertions.
2. The runtime-sentinel teardown scan passes for every test case.
3. Repository secret scanning and existing security gates remain green.
4. The no-cgo provider reports `managed delegated authentication unavailable on this build` and does not activate a local fallback.

## Operator-only evidence runbook (T1/T2)

**Preconditions:** #43 is merged; the automated CI gates above are green; the
approved test-tenant profile and designated test BWS record exist; the operator
has confirmed the selected tenant is non-production. Do not proceed if any
precondition is false.

### T1 — silent renewal after access-token expiry

1. Record UTC start time, cb365 commit/version, OS/build variant, profile label
   and the BWS record generation **only**.
2. Perform the approved test-tenant delegated login through the normal UI; do
   not paste a credential, cache value or BWS access token into the transcript.
3. Wait for the access token to expire according to the test-tenant policy.
4. Run the approved non-interactive `auth status` evidence command with output
   redaction enabled. Confirm zero interaction, successful renewal, a later
   expiry summary and a higher BWS record generation.
5. Paste a redacted outcome into #37 using the template below. Do not attach
   raw logs, `env`, process listings, cache files or BWS responses.

### T2 — safe failure after Entra session revocation

1. Record UTC start time, cb365 commit/version, OS/build variant, profile label
   and current BWS generation only.
2. The authorised test-tenant operator revokes sessions for the dedicated test
   account in Entra. This is a test-tenant action only.
3. After documented propagation time, run the same non-interactive evidence
   command once. Do not retry it in a loop.
4. Confirm a non-zero exit, clear re-login guidance, zero device-code prompts,
   no retry loop and no bearer/cache material in the redacted output.
5. Paste a redacted outcome into #37 using the template below. Re-authenticate
   only under a separately approved test-tenant cleanup step.

### #37 evidence comment template

```text
T[1|2] managed delegated-cache evidence
UTC: <RFC3339 UTC>
cb365 version/commit: <version or short SHA>
build: <OS, architecture, cgo state>
test profile label: <non-secret label>
record generation: <integer before -> integer after, where applicable>
result: <pass/fail; stable outcome class>
interaction/device prompt: <none|observed>
Entra/workload request observation: <expected outcome only>
redaction/sentinel scan: <pass/fail>
notes: <secret-free operational observation>
```

## Handoff conditions

- **Code (Cody):** #43 can implement the seams and T3–T6 contract now that the
  v2 design is accepted.
- **Test (Vera):** accepts T3–T6 only when CI demonstrates every row above,
  including the sentinel scan and zero-request invariants.
- **Operate (Knox):** validates the security boundary and reviewed redaction
  artefacts; no live BWS values are needed for this review.
- **Test-tenant operator:** performs T1/T2 only after #43 and the automated
  gates are complete, then records the redacted evidence on #37.

## Out of scope

- Production profiles, production BWS records and production Entra sessions.
- Changes to app-only authentication or tenant permissions.
- BWS CLI use for delegated cache reads/writes.
- Any storage, transcript or CI artefact containing delegated bearer material.
