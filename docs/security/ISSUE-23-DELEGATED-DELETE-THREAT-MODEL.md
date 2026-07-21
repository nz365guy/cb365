# Issue 23 delegated channel-message soft-delete threat model

Status: security design implemented in PR #55; release remains gated by the
owner-assisted #44 T1/T2 and test-tenant soft-delete evidence.

Source revision: `1871c14808741ad824e1e4f85177e82d4eaf7c0f`

## Overview

Issue 23 adds one destructive Microsoft Graph operation:

```text
cb365 teams channels delete-message \
  --team <team> --channel <channel> --message <message> --confirm
```

The approved scope is a delegated work-or-school account soft-deleting its own
Teams channel message. Chat messages, replies, batch deletion, permanent
deletion, application permissions and service-principal deletion are out of
scope.

Current delivery evidence (2026-07-21): #43 delivered the BWS EU-only
`cb365.msal-cache/v2` provider, PR #54 merged the credential-free T3–T6 suite,
and PR #55 implements exact-target deletion plus integrity-protected send
provenance. No Entra consent, BWS credential, tenant operation, production
profile, or live deletion was used in those implementation slices. T1/T2 and
one ordinary-member/team-owner test-tenant demonstration remain release gates.

The Graph operation is `POST
/teams/{team-id}/channels/{channel-id}/messages/{message-id}/softDelete`. It
requires delegated `ChannelMessage.ReadWrite`; Microsoft documents application
permission as unsupported. A successful operation returns HTTP 204.

Microsoft documents a different, broader delegated permission for reading a
channel message: `ChannelMessage.Read.All` (or the legacy `Group.Read.All` /
`Group.ReadWrite.All` alternatives). The delete design must not add one of
those permissions solely to perform an ownership pre-read. Own-message
enforcement is therefore a separate provenance gate, defined below, rather
than an assumed `/me` plus message-GET preflight.

Relevant implementation evidence at the source revision:

- `internal/auth/credential.go` uses Azure Identity device-code credentials,
  authentication records and a persistent cache for delegated sessions.
- `internal/auth/credential_cache_other.go` creates Azure Identity's local
  persistent cache and otherwise falls back to memory.
- `internal/auth/credential_cache_darwin.go` uses memory-only caching in release
  builds.
- `internal/auth/keyring.go` and `internal/auth/store_file.go` store profile
  material in an OS keyring or a locally encrypted file. They are not a BWS EU
  backing store for Azure Identity's delegated refresh cache.
- `cmd/cb365/teams.go` already requires `--confirm` for channel-message sends,
  but has no delete command.

Mark's approval for issue 23 requires delegated refresh material to be stored
only in Bitwarden Secrets Manager EU (BWS EU). The current cache path therefore
does not satisfy the approved storage boundary. Mark subsequently limited issue
20 to HTML-only delivery, so issue 20 is no longer the delegated-cache
dependency. A separately tracked delegated-auth delivery item must establish
and verify that boundary before issue 23 application code can be implemented.

## Threat Model, Trust Boundaries, and Assumptions

### Protected assets

- Delegated refresh tokens and Azure Identity/MSAL cache state.
- The runtime BWS credential used to retrieve or update that state.
- Short-lived access tokens and authentication records.
- Tenant, client and profile bindings that select an identity.
- Team, channel and message identifiers selected for deletion.
- The user's Teams channel messages and the integrity of other users' messages.
- Audit evidence that records who requested which operation and its result,
  without recording message content or credentials.

### Actors and attacker capabilities

- An authenticated delegated user operating the CLI legitimately.
- An app-only profile or automation process that must never reach this
  destructive endpoint.
- A local same-user process able to invoke the CLI, alter arguments or inspect
  ordinary process output.
- An attacker who obtains a profile file, authentication record, local cache or
  BWS bootstrap credential.
- Microsoft Entra ID and Microsoft Graph, which are trusted to authenticate and
  make the final authorisation decision.

The design does not assume that possession of a local profile proves the human
user's current intent, that a decoded token is authentic without verification,
or that a timed-out POST did not reach Graph.

### Trust boundaries

1. Shell and CLI arguments into Cobra command parsing.
2. Local profile metadata into delegated or app-only credential selection.
3. The process boundary into BWS EU and back into process memory.
4. BWS-backed cache state into Azure Identity/MSAL token acquisition.
5. The CLI process into Microsoft Entra ID.
6. The authenticated delegated identity into Microsoft Graph.
7. Graph responses into human-readable output and structured audit events.

### Security assumptions

- BWS EU is already available and creates no new SaaS spend.
- Its runtime credential is injected by the existing approved secret-delivery
  path, is least-privileged to the cb365 token records, and is never persisted
  in the repository, profile configuration, command line or logs.
- If the BWS CLI is used, its own authentication state is disabled or confined
  to an approved volatile path; the default persistent state directory is not
  silently accepted. A direct supported SDK is preferred over parsing secret
  values from child-process output.
- Microsoft Graph remains the final authorisation authority. Client-side checks
  are defence in depth and cannot widen Graph permissions.
- `ChannelMessage.ReadWrite` is the exact destructive delegated permission.
  `ChannelMessage.Read.All`, `Group.Read.All` and `Group.ReadWrite.All` are not
  acceptable additions solely to inspect the target before deletion.

### Supported cache architecture constraint

At the source revision, cb365 uses Azure Identity for Go v1.14.0 and its cache
module v0.4.0. The public `azidentity.Cache` type is an alias with unexported
state, and `azidentity/cache.New` constructs only the module's cross-platform
local persistent cache. It does not provide a public external-store interface
that can be implemented by a BWS adapter.

The underlying MSAL Go public-client library does expose
`cache.ExportReplace`, and `public.WithCache` accepts an externally managed
cache. The next design phase must therefore either migrate the delegated
device-code and silent-acquisition path to supported MSAL public-client APIs
with a BWS-backed `ExportReplace` implementation, or document and prove another
vendor-supported architecture that keeps all delegated refresh/cache material
inside BWS EU. Copying private Azure Identity cache internals or scraping its
local cache files is prohibited.

## Required security invariants

### Command and authorisation controls

1. The command accepts one exact team, channel and root-message identifier.
   Wildcards, lists, batches, chat-message IDs and reply IDs are rejected.
2. Identifiers are non-empty, length-bounded and reject NUL, CR and LF. URL
   construction uses the Graph SDK or escaped path-segment helpers rather than
   raw string concatenation.
3. `--confirm` is mandatory and binds the resolved profile, team, channel and
   message IDs shown to the user. The command must not resolve a different
   target after confirmation.
4. `--dry-run` performs local validation and prints the bound target, but does
   not construct a credential, acquire a token or make an HTTP request.
5. App-only mode fails before Graph-client construction. Tests must prove that
   app-only, unconfirmed and dry-run invocations make zero Entra ID and Graph
   requests.
6. The selected profile must be delegated and must request only the scopes
   needed for the operation, including `ChannelMessage.ReadWrite`.

### Ownership enforcement

7. A user-supplied message ID is not proof that the authenticated user authored
   the message. Before implementation, the delegated-auth delivery item must
   establish one supported own-message proof that does not add
   `ChannelMessage.Read.All` or either legacy Group permission merely for a
   pre-read. The preferred design limits deletion to message IDs recorded when
   cb365 successfully sent the message, bound to the same tenant, client,
   home-account/profile, team and channel in an integrity-protected provenance
   record.
8. If a vendor-documented author lookup becomes available under the already
   approved permissions, it may replace provenance only after a test-tenant
   proof. `/me` plus a channel-message GET is not assumed to work: `/me`
   requires `User.Read`, while Microsoft currently documents channel-message
   GET with `ChannelMessage.Read.All` rather than `ChannelMessage.ReadWrite`.
9. Missing, application-authored, cross-profile, cross-tenant or mismatched
   provenance fails closed before the soft-delete POST. Unverified JWT claims,
   display names, email addresses and CLI-supplied owner values are not
   ownership evidence.
10. Graph remains the final enforcement point. HTTP 401, 403 and 404 responses
    are failures and their bodies are not echoed if they could expose message
    content or tenant details. A provenance match never authorises a retry or a
    different target.

### Token storage, refresh and revocation

11. Delegated refresh/cache material for a delete-enabled profile is read from
    and written to BWS EU only. Azure Identity's local persistent cache, the OS
    keyring, the encrypted-file fallback and plaintext or environment-variable
    token storage are prohibited for that material.
12. Cache records are bound to tenant, client ID, account/home-account ID and
    profile. A record for one binding must not be accepted for another.
13. Refresh updates are serialised per profile and committed atomically. A
    stale or concurrent writer must fail without overwriting newer cache state.
    Newly returned refresh material is persisted before obsolete local state is
    discarded.
14. Tokens and BWS secret values exist only in process memory for the shortest
    practical period and are never printed, included in errors or sent to audit
    logs.
15. The BWS machine account has read/write access only to the dedicated cb365
    cache project or records. BWS client authentication state is opted out or
    held on an approved volatile filesystem and is covered by the same
    redaction tests as the delegated cache.
16. Local logout deletes the BWS cache record, authentication record and any
    legacy local cache for that profile, then verifies absence. A partial delete
    fails visibly and does not report success.
17. Local deletion is not described as server-side revocation. The runbook must
    separately document Entra revocation/administrator sign-out for a lost or
    compromised refresh token and explain that rotated refresh tokens can leave
    older tokens valid until revoked or expired.

### Request, retry and audit controls

18. The soft-delete POST is issued exactly once after all local and ownership
    checks pass. Generic HTTP retry middleware is disabled for this operation.
19. HTTP 204 is success. A transport timeout or dropped response is an
    ambiguous outcome: report that state and require a read-back check; do not
    automatically repeat the POST.
20. The audit event records timestamp, operation, tenant, profile identifier or
    pseudonym, resolved team/channel/message IDs, result class, HTTP status and
    Graph correlation ID when present. It never records token/cache material,
    the BWS credential, message content, display names or response bodies.
21. Audit failure is visible. It must not cause the command to repeat an
    already-issued soft-delete request.

## Attack Surface, Mitigations, and Attacker Stories

### Attack surface and mitigations

| Attack surface | Material threat | Required mitigation |
| --- | --- | --- |
| CLI arguments | Target substitution, newline/log injection, accidental destructive action | Strict validation, exact target binding, explicit confirmation, dry-run |
| Profile selection | App-only or wrong-tenant credential reaches destructive code | Fail before client construction; bind cache to tenant/client/account/profile |
| BWS boundary | Refresh-token or bootstrap-credential disclosure | Least privilege, memory-only secret handling, complete output/error redaction |
| Cache refresh | Lost update reintroduces stale refresh material | Per-profile serialisation, atomic commit and stale-writer rejection |
| Ownership/provenance | Another user's, another profile's or application-authored message is selected | Integrity-protected cb365 send provenance under the same identity and target binding; no permission widening for a pre-read |
| Graph POST | Duplicate destructive request after ambiguous failure | Exactly-once invocation in-process; no automatic POST retries; read-back guidance |
| Audit/output | Tokens or message content leave the process | Metadata-only schema and negative redaction tests |
| Logout/revocation | Local logout leaves usable material behind | Delete and verify every cache layer; distinguish local deletion from Entra revocation |

### Attacker stories

1. A script selects an app-only profile and invokes delete with `--confirm`.
   The command rejects it before any network request.
2. A local attacker swaps a message ID after the prompt. The confirmed target
   tuple is immutable, so the request cannot move to the substituted message.
3. A delegated user targets another member's message or a message absent from
   the same-profile cb365 send provenance. The command fails before the POST;
   Graph remains the final backstop for every accepted target.
4. A transport timeout occurs after Graph receives the POST. The CLI reports an
   ambiguous result and does not retry, preventing an uncontrolled duplicate.
5. Two processes refresh the same account concurrently. The stale writer is
   rejected instead of overwriting the newer BWS cache record.
6. A support bundle captures verbose logs. Negative tests ensure it contains no
   access token, refresh/cache material, BWS value or message content.
7. A compromised refresh token is removed locally. The operator is directed to
   the distinct Entra revocation procedure rather than being given false
   assurance that local deletion invalidated every server-side token.

## Severity Calibration (Critical, High, Medium, Low)

- **Critical:** access/refresh token or BWS bootstrap-credential disclosure;
  app-only bypass that sends a destructive request; arbitrary deletion outside
  the authenticated user's ownership; or addition of substantially broader
  Graph permissions.
- **High:** confirmation or dry-run bypass; cross-profile/tenant cache
  confusion; stale refresh state overwriting a newer credential; ownership
  provenance bypass; or logout silently retaining usable delegated cache state.
- **Medium:** message content or identifiers exposed beyond the metadata audit
  contract; blind retry after an ambiguous response; target-binding race; or
  missing security audit evidence.
- **Low:** overly detailed but non-secret errors, incomplete operator guidance,
  or documentation drift that does not change the enforced control.

## Delegated-cache and release security gate

The former implementation dependency is no longer a Blocked board state: #43
and PR #54 supply the provider and automated evidence. The following controls
remain the complete release gate; the owner-assisted items must still pass
before this feature is treated as live-ready. Issue 20 cannot satisfy this
gate because Mark selected HTML-only scope for that issue:

1. A documented Azure Identity/MSAL cache architecture whose delegated
   refresh/cache material is backed only by BWS EU for delete-enabled profiles.
   It must reconcile the repository's current OS-keyring policy with Mark's
   BWS-only approval, use a vendor-supported external-cache extension point,
   remove silent fallback to any unapproved persistent store, and prevent the
   BWS client from creating an unmanaged persistent authentication state file.
2. A versioned record schema binding tenant, client, home-account/profile and
   cache payload, plus per-profile locking, atomic refresh rotation and
   stale-writer rejection.
3. A least-privilege runtime path for the BWS credential with no repository,
   profile, command-line, environment-dump, output or log exposure.
4. Login, silent-refresh, concurrent-refresh, expiry, BWS-unavailable, malformed
   record, tenant mismatch, logout and legacy-cache migration tests. Failures
   must fail closed and must not send a destructive Graph request.
5. `auth status` evidence that reports the cache backend and freshness without
   disclosing credential material.
6. Logout evidence that removes and verifies BWS, authentication-record and
   legacy cache state, plus an operator-tested Entra revocation runbook.
7. An own-message proof compatible with the exact approved Graph permission.
   Prefer an integrity-protected record created from cb365's successful send
   response and bound to tenant, client, home-account/profile, team, channel and
   message. Do not add `ChannelMessage.Read.All` or legacy Group permissions
   solely for ownership inspection. Test both an ordinary member and a team
   owner under the applicable Teams messaging policy.
8. A recording-transport security suite for issue 23 proving zero requests for
   unconfirmed, dry-run and app-only paths; no POST on provenance or token
   failure; exactly one POST on an authorised path; redaction; HTTP 204; safe
   401/403/404 handling; and no automatic duplicate after a timeout.

Meeting this gate authorises implementation review; it does not itself authorise
tenant consent. Adding delegated `ChannelMessage.ReadWrite` requires its normal
Microsoft Entra consent process and must not be applied as an unattended live
auth mutation.

## Primary references

- [Microsoft Graph: soft-delete a chatMessage](https://learn.microsoft.com/en-us/graph/api/chatmessage-softdelete?view=graph-rest-1.0)
- [Microsoft Graph: get a channel chatMessage](https://learn.microsoft.com/en-us/graph/api/chatmessage-get?view=graph-rest-1.0)
- [Microsoft Graph permissions reference](https://learn.microsoft.com/en-us/graph/permissions-reference)
- [Microsoft Teams messaging policies](https://learn.microsoft.com/en-us/microsoftteams/messaging-policies-in-teams)
- [Azure Identity for Go](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/azidentity)
- [Azure Identity cache module for Go](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache)
- [MSAL Go externally managed cache contract](https://pkg.go.dev/github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache)
- [MSAL Go public-client `WithCache`](https://pkg.go.dev/github.com/AzureAD/microsoft-authentication-library-for-go/apps/public#WithCache)
- [Microsoft identity platform refresh tokens](https://learn.microsoft.com/en-us/entra/identity-platform/refresh-tokens)
- [Microsoft identity platform device-code flow](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-device-code)
- [Bitwarden Secrets Manager CLI](https://bitwarden.com/help/secrets-manager-cli/)
- [Bitwarden Secrets Manager access tokens](https://bitwarden.com/help/access-tokens/)
- [Issue 20 HTML-only scope decision](https://github.com/nz365guy/cb365/issues/20#issuecomment-5019321562)

Repository: efbcc95779452b3966e309a5ff637d95d60e7eb112be891e7cebebabef81bafa
Version: 1871c14808741ad824e1e4f85177e82d4eaf7c0f
