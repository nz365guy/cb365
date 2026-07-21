# Requirements: safeguarded Teams channel-message soft-delete

> **Source issue:** [#23](https://github.com/nz365guy/cb365/issues/23)  
> **Status:** Requirements Complete — approved scope; Code is dependency-gated  
> **Author:** Scout 🔍 · **Date:** 2026-07-21 (Pacific/Auckland)  
> **Security design:** [`ISSUE-23-DELEGATED-DELETE-THREAT-MODEL.md`](../security/ISSUE-23-DELEGATED-DELETE-THREAT-MODEL.md) (PR #38)  
> **Shared dependencies:** #43 managed delegated-cache provider; #44 T1–T6 evidence

## 1. Validated problem statement

An authenticated work-or-school user needs to retract a Teams **root channel message that cb365 recorded as their own**, without granting app-only deletion, reading other users' messages, or exposing delegated credentials.

- **Mark sign-off:** confirmed in [#23](https://github.com/nz365guy/cb365/issues/23#issuecomment-5018648424) on 2026-07-20: safeguarded delegated-only option approved.
- **Authoritative API evidence:** Microsoft Graph's `softDelete` channel-message endpoint is `POST /teams/{team-id}/channels/{channel-id}/messages/{message-id}/softDelete`, returns `204 No Content`, requires delegated work-or-school `ChannelMessage.ReadWrite`, and provides no application permission. [Microsoft Learn](https://learn.microsoft.com/en-us/graph/api/chatmessage-softdelete?view=graph-rest-1.0)

## 2. Requirements

| ID | Type | Pri | Requirement | Source | Rationale | Acceptance criteria | Verify | Status |
|---|---|---|---|---|---|---|---|---|
| REQ-2301 | Functional | Must | `cb365 teams channels delete-message --team <id> --channel <id> --message <id> --confirm` shall soft-delete exactly one root channel message with one Graph POST to the v1.0 `softDelete` endpoint. | Mark approval; Graph documentation | Provides the approved retraction capability while preventing batch or permanent deletion. | 1) Valid delegated fixture receives HTTP 204 and reports secret-free success. 2) Lists, wildcards, chat IDs and reply IDs are rejected before token acquisition. | Test | Ready |
| REQ-2302 | Functional | Must | The command shall require `--confirm` and bind the resolved profile, team, channel and message tuple before constructing a credential or HTTP client. | Mark approval; PR #38 invariant 3 | Prevents accidental or target-substitution deletion. | 1) Missing `--confirm` makes zero Entra and Graph requests. 2) A post-confirmation target change is rejected and produces no POST. | Test | Ready |
| REQ-2303 | Functional | Must | `--dry-run` shall validate and display only the bound target identifiers, then make zero credential, Entra or Graph requests. | PR #38 invariants 3–5 | Lets an operator verify a destructive target without accessing credentials. | 1) Dry run emits the exact input tuple. 2) Recording transports show zero requests and credential factory calls. | Test | Ready |
| REQ-2304 | Functional | Must | The command shall accept only a delegated work-or-school profile with `ChannelMessage.ReadWrite`; app-only or missing-scope profiles shall fail before Graph-client construction. | Mark approval; Graph documentation; PR #38 invariants 5–6 | The API has no supported application permission. | 1) App-only invocation makes zero Entra and Graph requests. 2) Missing scope returns an actionable delegated-scope error without a POST. | Test | Ready |
| REQ-2305 | Functional | Must | Before the POST, the command shall verify an integrity-protected cb365 send-provenance record bound to the same tenant, client, home-account/profile, team, channel and root message ID. | PR #38 invariants 7–10 | `ChannelMessage.ReadWrite` does not justify a broader message-read permission for ownership inspection. | 1) A matching provenance record permits one POST. 2) Missing, altered, cross-profile, cross-tenant, app-authored or mismatched provenance makes zero POSTs. | Test | Ready |
| REQ-2306 | Functional | Must | The command shall issue no automatic retry after a soft-delete POST; HTTP 204 is success, while timeout, 401, 403, 404 and other failures return a redacted result class and no response body. | PR #38 invariants 18–19 | A timeout may mean the destructive request reached Graph; retry could cause unintended repeat effects. | 1) Recording transport observes one POST for each authorised invocation. 2) A timeout emits ambiguous-outcome guidance and observes no second POST. | Test | Ready |
| REQ-2307 | Non-functional | Must | The command shall write a metadata-only audit event containing timestamp, operation, tenant, profile pseudonym, team/channel/message IDs, result class, HTTP status and Graph correlation ID when present. | PR #38 invariant 20 | Supports traceability without retaining protected content. | 1) Successful and failed fixtures contain all permitted fields. 2) Audit data contains no message body, display name, token, cache bytes or BWS value. | Test | Ready |
| CON-2301 | Constraint | Must | Delete-enabled profiles shall use only the accepted `cb365.msal-cache/v2` BWS EU delegated-cache provider from #43; no OS keyring, encrypted file, Azure Identity local cache, plaintext file or environment-variable token fallback is permitted. | Mark approval; ADR-0057; #43 | Delegated refresh material is a high-value credential and must remain in the approved store. | 1) #43's fail-closed provider and migration/logout tests are merged. 2) Provider unavailability or binding failure makes zero Graph POSTs. | Inspection + Test | Ready |
| CON-2302 | Constraint | Must | The feature shall not add `ChannelMessage.Read.All`, `Group.Read.All` or `Group.ReadWrite.All` solely to inspect message ownership. | PR #38; Graph permission documentation | Avoids widening consent beyond the approved destructive operation. | 1) Manifest/scope inspection finds none of the prohibited permissions added for #23. 2) Ownership tests rely on provenance, not a channel-message GET. | Inspection + Test | Ready |
| CON-2303 | Constraint | Must | Implementation shall add no paid service, hard-coded secret, tenant-consent mutation or production-profile operation. BWS EU machine credentials shall use the approved runtime injection path only. | Mark approval; ADR-0057 | Preserves the approved cost and secret boundary. | 1) Repository and CI-secret scans are green. 2) PR evidence shows no live tenant, production profile or BWS value was accessed. | Inspection | Ready |

## 3. Non-functional checklist — ISO/IEC 25010:2023

1. **Functional suitability** — Requirements REQ-2301 to REQ-2306 cover the one approved root-message action, confirmation, ownership proof and safe outcomes; chat, replies and permanent deletion are excluded.
2. **Performance efficiency** — Local rejection paths complete before credential construction; the authorised path performs at most one Graph POST. No latency SLO is imposed because Graph latency is external; integration tests must assert request count, not elapsed wall time.
3. **Compatibility** — Uses Microsoft Graph v1.0 `softDelete` and delegated work-or-school `ChannelMessage.ReadWrite`. App-only behaviour stays unchanged and refuses the command.
4. **Interaction capability** — The CLI requires an explicit `--confirm`; dry-run shows the exact target identifiers. Errors state the remediation class without exposing message content or credentials.
5. **Reliability** — No automatic destructive retry. A timeout is reported as ambiguous and requires read-back guidance; 401/403/404 are final failures. Shared cache availability, refresh and logout must meet #43/#44 evidence gates before release.
6. **Security** — BWS EU-only delegated cache, least privilege, provenance binding, strict identifier validation, redacted errors/audits and zero-request negative-path tests are mandatory.
7. **Maintainability** — Keep provenance validation, credential selection, Graph transport and audit emission behind testable interfaces. Add focused unit tests plus recording-transport integration tests; maintain existing `go test ./...`, `go vet ./...`, CI security and CodeQL gates.
8. **Flexibility** — The command is CLI-only and uses existing Go/Cobra and Microsoft Graph SDK patterns. It must fail closed on unsupported no-cgo builds as #43 specifies, without a local-cache replacement.
9. **Safety** — N/A for physical safety. Digital-harm controls are addressed through confirmation, provenance and non-retry requirements.

**Conflict noted:** a Graph message GET could provide an ownership preflight but requires broader read consent; the approved provenance approach prioritises least privilege and fails closed when provenance is unavailable. This is resolved by the accepted PR #38 threat model, not silently by implementation.

## 4. Surface and deployment

- **Who uses it / context:** authenticated internal operators using the cb365 CLI; no external UI or scheduled automation is authorised.
- **Recommended surface:** CLI command under `teams channels`, using the existing Cobra command structure and Microsoft Graph SDK.
- **Deployment target:** existing supported cb365 build targets. No hosted service, new infrastructure or SaaS is introduced.

## 5. Release gates and dependencies

Code may implement the command contract only after #43 exposes the accepted BWS-backed delegated provider and provenance-supporting send path. Release requires all of the following evidence:

1. #43 merged with BWS-only cache binding, fail-closed migration/logout/revocation behaviour and supported build handling.
2. #44's T3–T6 automation green in CI and secret-free T1/T2 test-tenant evidence recorded on #37.
3. A dedicated #23 recording-transport suite proving every REQ-2301–REQ-2307 negative and authorised path, including zero-request, no-retry and redaction assertions.
4. A test-tenant demonstration of an authenticated user's own root-message soft-delete after normal Entra consent; no production tenant operation is included in this work item.

## 6. Out of scope

- Chat messages, replies, batch or wildcard deletion, permanent delete and undo.
- Application/service-principal deletion and any app-only behaviour change.
- Message ownership reads that require additional Graph permissions.
- Production tenant consent, production profile operations, BWS value inspection and delegated credential migration.
- Teams HTML rendering (#20) and shared cache/provider implementation (#43).

## 7. Budget impact

- **Token cost:** no AI token consumption at runtime.
- **Ongoing compute/hosting:** no incremental cost; the command uses existing cb365 execution and Microsoft Graph/BWS EU services.
- **Budget controls:** soft alert NZ$0.00 (70%) and hard pause NZ$0.00 (100%) for new SaaS spend; any proposed paid dependency requires a new decision.
- **Azure MVP credit coverage:** not applicable; no Azure hosting workload is added.

## 8. Architecture status and handoff

- **New ADR required:** No. The security architecture is already accepted in PR #38 and the shared delegated-cache architecture in ADR-0057. This document introduces no new architectural choice.
- **Plan outcome:** complete. The implementation-facing requirements are ready, but delivery remains dependency-gated by #43 and #44.
