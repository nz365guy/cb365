# cb365 — Session Handoff

> **Last updated:** 2026-04-03
> **Session:** Initial architecture, research, and Phase 0 scaffold
> **Next session starts here**

---

## What Is cb365?

Enterprise CLI for Microsoft 365 via Microsoft Graph. Built in Go on Microsoft's official `msgraph-sdk-go` and `azidentity` libraries. Entra ID authenticated. Designed for agent consumption (OpenClaw/OpenCLAW) with `--json` output.

**Two repos, both private under `nz365guy`:**
- **`nz365guy/cb365`** — The canonical, clean, enterprise-grade CLI. Eventually goes public. No Cloverbase-specific config.
- **`nz365guy/cb365-internal`** — Cloverbase working copy with Rook skills, tenant shortcuts, deployment scripts. Never goes public.

---

## Strategic Context

This tool exists because:
1. Microsoft retired their Graph CLI (Aug 2025), pushing to PowerShell only
2. MOG (`jaredpalmer/mogcli` v0.0.2) covers Mail/Calendar/Contacts/Tasks/OneDrive but NOT Planner, Forms, SharePoint, or To Do list management
3. Microsoft's new MCP servers for M365 (Work IQ) require Copilot licence ($30/user/month) and are preview-only
4. OpenClaw (345K+ GitHub stars) has no first-class M365 integration
5. Mark's Cloverbase BOS needs Rook to manage Microsoft To Do tasks

**Key risk identified during stress-testing:** Microsoft is building MCP-native M365 access via Agent 365 / Work IQ (GA May 1, 2026 with E7 licence). cb365 may have a 12-18 month window before Microsoft's own tooling covers the same surface. Strategy: build private-first, use internally, go public only when confident.

---

## Current State — Phase 0 (Foundation)

### What's Built and Compiling

```
~/cb365/
├── .github/workflows/ci.yml    ← Build + gosec + govulncheck
├── LICENSE                      ← MIT
├── README.md
├── SECURITY.md                  ← Responsible disclosure policy
├── HANDOFF.md                   ← This file
├── cmd/cb365/
│   ├── main.go                  ← CLI entrypoint
│   ├── root.go                  ← Root command, global flags
│   └── auth.go                  ← auth login/status/logout/profiles/use
├── go.mod                       ← Go 1.24, azidentity, cobra, go-keyring
├── go.sum
└── internal/
    ├── auth/
    │   ├── auth.go              ← Device-code flow, JWT decode
    │   └── keyring.go           ← OS keychain token storage
    ├── config/
    │   └── config.go            ← Profile management, ~/.config/cb365/
    └── output/
        └── output.go            ← JSON/plain/table formatters
```

### Working Commands

| Command | Status | Notes |
|---------|--------|-------|
| `cb365 --help` | ✅ | Shows all commands and global flags |
| `cb365 version` | ✅ | `0.1.0-dev` |
| `cb365 auth login` | ✅ Compiles | Needs live test against Entra tenant |
| `cb365 auth status` | ✅ Compiles | JWT decode for display, no raw token in output |
| `cb365 auth logout` | ✅ Compiles | Clears keychain + config |
| `cb365 auth profiles` | ✅ Compiles | Lists all configured profiles |
| `cb365 auth use [name]` | ✅ Compiles | Switches active profile |

### Global Flags

`--json`, `--plain`, `--profile`, `--verbose`, `--dry-run`

### Dependencies

| Package | Purpose |
|---------|---------|
| `azure-sdk-for-go/sdk/azidentity` v1.13.1 | Entra ID auth (device-code, client creds) |
| `azure-sdk-for-go/sdk/azcore` v1.21.0 | Token types, HTTP pipeline |
| `spf13/cobra` v1.10.2 | CLI framework |
| `zalando/go-keyring` v0.2.8 | OS keychain (macOS/Linux/Windows) |

---

## What Needs To Happen Next (Phase 0 Completion)

### Priority 1: Keychain Fallback for Headless Linux

**Problem:** The VM (`vm-openclaw-01`) runs headless Ubuntu with no desktop session. `go-keyring` uses `secret-service` (D-Bus) which requires a running keyring daemon. On a headless server, this will fail.

**Options to evaluate:**
1. `go-keyring` with `keyctl` backend (Linux kernel keyring — works headless)
2. Encrypted file backend with passphrase from env var (`CB365_KEYRING_PASSWORD`)
3. `pass` (GPG-based password store) as backend
4. Set env var `KEYRING_BACKEND=file` to force file-based fallback

**Decision needed:** Which fallback pattern? MOG uses plaintext file with 0600 perms (insecure). We should do better.

### Priority 2: Live Auth Test

Run against the Cloverbase Entra tenant:
```bash
cd ~/cb365
./cb365 auth login \
  --tenant 0ab1b54d-8a10-46ed-9a90-ba1f3a6e87d5 \
  --client 22809b82-c9ab-4ed8-82e0-e69507b251b4 \
  --scopes Tasks.ReadWrite \
  --name work-delegated
```

This will produce a device-code URL that Mark must visit to complete auth. After auth, verify:
```bash
./cb365 auth status --json
```

### Priority 3: Install Binary

```bash
cd ~/cb365
go build -o ~/.local/bin/cb365 ./cmd/cb365/
cb365 version  # verify it's in PATH
```

### Priority 4: Commit and Push

The `token.Value` → `token.Token` fix from this session needs committing:
```bash
cd ~/cb365
git add -A
git commit -m "fix: AccessToken field name (Token not Value)"
git push
```

---

## Phase 0.5 — App-Only Auth (Next After Phase 0)

Add `cb365 auth login --mode app-only --client-secret` for unattended Rook jobs. Uses `azidentity.ClientSecretCredential`. Secret stored in keychain, never in config. Required for scheduled 7am task review.

---

## Phase 1 — Microsoft To Do (After Phase 0.5)

Full CRUD for To Do lists and tasks. This is the first Graph workload and sets the pattern for all subsequent modules.

**New dependency needed:** `msgraph-sdk-go` (Microsoft's official Go Graph SDK)

**Commands to build:**
```
cb365 todo lists                              ← list all task lists
cb365 todo lists create --name "BOS Actions"  ← create list
cb365 todo lists update --list ID --name ...  ← rename list
cb365 todo lists delete --list ID             ← delete list
cb365 todo tasks list --list "Cloverbase"     ← list tasks (name OR ID)
cb365 todo tasks get --list X --task Y        ← get single task
cb365 todo tasks create --list X --title ...  ← create with due date, body
cb365 todo tasks update --list X --task Y     ← update
cb365 todo tasks complete --list X --task Y   ← mark complete
cb365 todo tasks delete --list X --task Y     ← delete
```

**Entra scopes:** `Tasks.ReadWrite` (already consented on Cloverbase tenant)

**Integration with Rook:** After Phase 1, create `~/.openclaw/workspace/skills/cb365/SKILL.md` teaching Rook how to use cb365 for tasks. Migrate 8 items from `~/.openclaw/workspace/projects/TASKS.md` into To Do "Cloverbase" list.

---

## Full Phase Plan

| Phase | What | Status |
|-------|------|--------|
| 0 | Auth foundation (delegated) | **Scaffolded, needs live test** |
| 0.5 | App-only auth (client credentials) | Not started |
| 1 | Microsoft To Do (full CRUD) | Not started |
| 2 | Mail + Calendar + Contacts (replace MOG) | Not started |
| 3 | Planner | Not started |
| 4 | SharePoint + OneDrive | Not started |
| 5 | Hardening for public release | Not started |

---

## Key Technical Decisions Made

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go | Security surface (single binary, no runtime deps), compiled = harder to tamper, azidentity handles auth |
| CLI framework | Cobra | Industry standard for Go CLIs |
| Auth library | `azidentity` | Microsoft's official, handles device-code + client creds + cert |
| Token storage | OS keychain via `go-keyring` | Never plaintext. Fallback needed for headless Linux |
| Graph SDK | `msgraph-sdk-go` (not yet added) | Microsoft's official, auto-generated from OpenAPI spec |
| Repos | Both under `nz365guy`, both private | No separate Cloverbase org |
| CLI name | `cb365` | Short, memorable, Cloverbase + M365 |

---

## Existing M365 Setup (Cloverbase Tenant)

| Item | Value |
|------|-------|
| Tenant ID | `0ab1b54d-8a10-46ed-9a90-ba1f3a6e87d5` |
| Client ID | `22809b82-c9ab-4ed8-82e0-e69507b251b4` |
| App name | `Cloverbase-mogcli-automation` |
| UPN | `mark.smith@cloverbase.com` |
| Consented scopes | Calendars.RW, Contacts.RW, Mail.Read, Mail.Send, Tasks.RW, User.Read |
| MOG profile | `work-delegated` (active), `work-app` (app-only) |
| MOG config | `~/.config/mogcli/config.json` |
| MOG binary | `~/go/bin/mog` (symlink `~/.local/bin/mog`) |
| Licence | E5 |

**Scopes needed for future phases:**
- Phase 3 (Planner): `Group.ReadWrite.All`
- Phase 4 (SharePoint): `Sites.ReadWrite.All`
- Phase 4 (OneDrive): `Files.ReadWrite.All`

---

## Microsoft To Do Lists (Live on Tenant)

| List | ID (truncated) | Shared | Owner |
|------|---------------|--------|-------|
| Tasks | AAMk...ESAAA= | No | Yes |
| Cloverbase | AAMk...T4AAA= | Yes | Yes |
| House Hold | AAMk...0oAAA= | Yes | Yes |
| nz365guy | AAMk...5XAAA= | No | Yes |
| Cloverbase Business Plan | AAMk...wKAAA= | Yes | No |
| Cloverbase Consults | AAMk...5gAAA= | Yes | No |
| Bar inventory | AAMk...CAAA= | Yes | Yes |
| HiTechHippies | AAMk...VAAA= | Yes | Yes |
| Flagged Emails | AAMk...T3AAA= | No | Yes |

**Current Cloverbase list:** 1 task ("Website", created 2023-04-03, not started)
**Current TASKS.md:** 8 active items to migrate

---

## Known Issues / Gotchas

1. **Azure NZ North IPv6 egress is broken** — All HTTPS calls from the VM need IPv4 forcing. For Go, this may need custom `http.Transport` with `DialContext` that resolves IPv4 only. Test this during live auth.

2. **go-keyring on headless Linux** — Will fail without `secret-service`. Must implement fallback before Phase 0 is complete.

3. **Rook is ROOK-ONLY for M365** — When cb365 replaces MOG, the same exclusivity rule applies. Update `SOUL.md`, `TOOLS.md`, and `AGENTS.md` across all 17 agent files.

4. **MOG token format** — MOG stores both access and refresh tokens in its keyring. cb365's `azidentity` handles token refresh internally via MSAL cache. The two systems are independent — no migration needed, just parallel operation during transition.

5. **Three-location config sync** — Does NOT apply to cb365. cb365 config lives only at `~/.config/cb365/config.json`. This is separate from OpenCLAW config.

---

## Files Changed This Session (Uncommitted)

```
cmd/cb365/auth.go  — Fixed token.Value → token.Token (azcore API)
```

**First action next session:** Commit and push this fix + HANDOFF.md.
