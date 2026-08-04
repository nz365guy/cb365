# cb365

**Scriptable access to Microsoft 365 from the command line.**

If you need to automate Microsoft 365 — create tasks, send mail, manage calendars, post to Teams, work with SharePoint, OneDrive, or Planner — you currently have two options: write a custom app against the Graph API, or click through the UI by hand. cb365 gives you a third: a single command-line tool that does it all, with structured output that scripts and AI agents can consume directly.

```bash
# List your tasks as JSON
cb365 todo tasks list --list "My Tasks" --json

# Create a calendar event with a Teams link
cb365 calendar create --subject @subject.txt --start "2026-04-10T10:00:00+12:00" \
  --end "2026-04-10T10:30:00+12:00" --attendee @attendees.txt --teams

# Send mail (with safety confirmation)
cb365 mail send --to @recipients.txt --subject @subject.txt --body @body.txt --confirm
```

58 commands across 10 workloads. One binary. Zero runtime dependencies. Built in Go.

> ⚠️ **Pre-release** — cb365 is `v0.1.0`. APIs may change. Review the [security design](#security) before use in production.

## Who Is This For?

- **DevOps and platform engineers** who need to script M365 operations (create tasks from CI, post build results to Teams, sync files to SharePoint)
- **AI agent builders** who need structured M365 access for autonomous workflows — cb365's `--json` output and safety flags were designed for agent consumption
- **IT admins** who want a CLI alternative to PowerShell for Graph API operations
- **Developers** who are tired of writing boilerplate Graph SDK code for simple M365 tasks

## Why Not Just Use the Graph API Directly?

You can. cb365 is a wrapper around the same Microsoft Graph API. But cb365 handles the parts that slow you down:

- **Authentication** — device-code, client credentials, and certificate flows. Tokens stored in your OS keychain, never in plaintext.
- **Safety** — 44 hardcoded rules. Deletes require `--force`. Broadcasts require `--confirm`. `--dry-run` on every write. Tokens never appear in output.
- **Output** — every command supports `--json`, `--plain` (TSV), and human-readable tables. Pipe to `jq`, `cut`, or feed directly to an AI agent.
- **Profiles** — manage delegated and app-only auth side by side. Switch tenants with `cb365 auth use`.
- **No dependencies** — single binary. No runtime, no Docker, no Python. Drop it on a server and it works.

---

## Quick Start

### Install

**From source (requires Go 1.24+):**

> **Note:** Distro-packaged Go (e.g. `apt install golang-go`) is often too old. Download Go 1.24+ from [go.dev/dl](https://go.dev/dl/) if your system version is below 1.24.

```bash
go install github.com/nz365guy/cb365/cmd/cb365@latest
```

**From release binaries:**

Download the latest release from [Releases](https://github.com/nz365guy/cb365/releases) for your platform. Verify the signature and extract:

```bash
# Linux (amd64)
tar xzf cb365_linux_amd64.tar.gz
sudo mv cb365 /usr/local/bin/

# macOS (Apple Silicon)
tar xzf cb365_darwin_arm64.tar.gz
sudo mv cb365 /usr/local/bin/

# Windows — extract cb365.exe and add to your PATH
```

### Register an Entra ID App (5 minutes)

1. Open the [Microsoft Entra admin center](https://entra.microsoft.com)
2. Navigate to **Identity → Applications → App registrations → New registration**
3. Configure:
   - **Name:** `cb365` (or your preference)
   - **Supported account types:** Accounts in this organizational directory only (single tenant)
   - **Redirect URI:** Leave empty
4. Click **Register**. Note your **Application (client) ID** and **Directory (tenant) ID**.
5. Go to **Authentication** → Enable **Allow public client flows** → Save
6. Go to **API permissions** → **Add a permission** → **Microsoft Graph** → **Delegated permissions**
7. Add the scopes you need (see [Scopes by Workload](#scopes-by-workload) below)
8. Click **Grant admin consent** if you are a tenant admin, or ask your admin to consent

> **Headless environment?** Delegated authentication uses Bitwarden Secrets Manager EU. For the app-only encrypted-file fallback, inject `CB365_KEYRING_PASSWORD` at process start from an approved secret manager; never place it in a shell profile, command line, repository, or service file.

### Authenticate (2 minutes)

```bash
# Delegated auth (interactive — you sign in via browser)
cb365 auth login \
  --tenant YOUR_TENANT_ID \
  --client YOUR_CLIENT_ID \
  --scopes Tasks.ReadWrite \
  --bws-organization YOUR_BWS_ORGANIZATION_ID \
  --bws-project YOUR_DEDICATED_BWS_PROJECT_ID \
  --name my-profile

# Follow the device-code prompt — open the URL, enter the code, sign in
```

### Use it

```bash
# Check auth status
cb365 auth status

# List your To Do task lists
cb365 todo lists list

# Create a task
cb365 todo tasks create --list @list-name.txt --title @task-title.txt --due 2026-04-15

# JSON output for scripting
cb365 todo tasks list --list "My Tasks" --json | jq '.[] | .title'
```

**That's it.** Zero to listing tasks in under 15 minutes.

---

## Authentication

cb365 supports three Entra ID authentication flows.

### Delegated (Device Code)

You sign in via browser. The token is scoped to your permissions. Best for interactive use.

```bash
cb365 auth login \
  --tenant TENANT_ID \
  --client CLIENT_ID \
  --scopes Tasks.ReadWrite,Mail.Read,Calendars.ReadWrite \
  --bws-organization BWS_ORGANIZATION_ID \
  --bws-project BWS_PROJECT_ID \
  --name work
```

Delegated bearer material is stored only in the profile-bound Bitwarden Secrets Manager EU record. The machine-account credential is accepted only from the injected `BWS_ACCESS_TOKEN` environment boundary; there is no CLI flag, local token-store, Azure Identity cache, or plaintext fallback. The initial managed target is Linux with cgo; unsupported builds fail closed.

Tokens auto-refresh silently through the BWS-backed MSAL cache, subject to Entra policy and revocation. For fully zero-touch workflows, use [app-only auth](#app-only-client-secret) instead.

To move the sole remaining legacy delegated profile on a host, run an explicit resumable migration:

```bash
cb365 auth migrate \
  --profile work \
  --bws-organization BWS_ORGANIZATION_ID \
  --bws-project BWS_PROJECT_ID
```

Migration verifies legacy ownership and modes before reading, proves the BWS write by readback, then removes and verifies every legacy layer. Workload commands remain disabled while cleanup is incomplete; rerun the same command to resume cleanup.

To rebind an encrypted-file app-only store created before profile-bound
ciphertext was introduced, select any affected app-only profile:

```bash
cb365 auth migrate --profile work-cert
```

Because the legacy file has one global format version, this operation validates
and migrates every app-only entry atomically. Each entry's token claims must
match its configured tenant and client, and it must contain exactly one usable
refresh credential. Ambiguous, orphaned, incomplete, or unknown-version
entries fail closed and require reauthentication.

### App-Only (Client Secret)

For unattended automation. The app authenticates with a client secret. Requires application permissions (not delegated) in Entra.

Client secrets are accepted from standard input only. Retrieve the value from
your managed secret store and pipe it directly to `cb365`; do not place the
secret in command arguments, shell history, environment files, or scripts.
Certificate authentication is preferred for unattended deployments.

The secret is stored encrypted in the OS keychain. Token auto-refreshes without human intervention.

### App-Only (Certificate)

Microsoft's recommended approach for production. The private key never leaves the machine.

```bash
cb365 auth login \
  --mode app-only \
  --tenant TENANT_ID \
  --client CLIENT_ID \
  --certificate /path/to/cert.pem \
  --name production
```

The PEM file must contain both the certificate chain and private key (RSA PKCS1/PKCS8 or EC).

### Managing Profiles

```bash
cb365 auth profiles          # List all profiles
cb365 auth use work          # Switch active profile
cb365 auth status            # Show current token info
cb365 auth logout --name old # Remove a profile
cb365 todo lists list --profile work  # One-off profile override
```

Managed delegated logout verifies the BWS record and legacy cache layers are absent. Entra session revocation is a separate administrator action.

> **Naming convention:** We recommend `work-delegated` for interactive profiles and `work-app` for app-only automation. This makes it clear at a glance which auth flow a profile uses.

---

## Commands

### Global Flags

Every command supports these flags:

| Flag | Description |
|------|-------------|
| `--json` | Output structured JSON to stdout |
| `--plain` | Output tab-separated values to stdout |
| `--profile NAME` | Override the active profile for this command |
| `--dry-run` | Preview write operations without executing |
| `--verbose` | Enable verbose logging (never includes tokens) |

Human-readable output goes to stderr. Machine-readable output (`--json`, `--plain`) goes to stdout. This means `cb365 mail list --json | jq .` works cleanly in pipelines.

### Keep business content out of process arguments

Any string flag accepts an indirect value: `@path` reads the value from a file
and `@-` reads it from stdin. Repeatable string flags read one non-empty value
per line. Use `@@value` only when the intended literal value begins with `@`.

Use indirect values for subjects, bodies, names, addresses, search terms,
tenant paths, and other private or business content. Literal flag values can be
retained in shell history, process listings, endpoint telemetry, and job logs.
For example:

```bash
printf '%s' 'Private subject' >subject.txt
printf '%s' 'Private body' >body.txt
cb365 mail send --to @recipients.txt --subject @subject.txt --body @body.txt --confirm
```

Protect and remove temporary input files according to your local policy. Do
not place credentials or access tokens in general command flags; authentication
secrets use the dedicated stdin/certificate flow described above.

### Auth

| Command | Description |
|---------|-------------|
| `cb365 auth login` | Authenticate with Entra ID |
| `cb365 auth status` | Display current token info (never shows raw tokens) |
| `cb365 auth logout` | Revoke and clear cached credentials |
| `cb365 auth profiles` | List all configured profiles |
| `cb365 auth use NAME` | Switch the active profile |

### To Do

| Command | Description |
|---------|-------------|
| `cb365 todo lists list` | List all task lists |
| `cb365 todo lists create --name @name.txt` | Create a task list |
| `cb365 todo lists update --list ID --name @name.txt` | Rename a task list |
| `cb365 todo lists delete --list ID` | Delete a task list |
| `cb365 todo tasks list --list "Name"` | List tasks (accepts name or ID) |
| `cb365 todo tasks get --list X --task Y` | Get a single task |
| `cb365 todo tasks create --list X --title @title.txt [--body @body.txt] [--due YYYY-MM-DD]` | Create a task |
| `cb365 todo tasks update --list X --task Y [--title/--status/--body/--due]` | Update a task |
| `cb365 todo tasks complete --list X --task Y` | Mark task complete |
| `cb365 todo tasks delete --list X --task Y` | Delete a task |

### Mail

| Command | Description |
|---------|-------------|
| `cb365 mail list` | List inbox messages |
| `cb365 mail get --id ID` | Get a single message |
| `cb365 mail send --to @recipients.txt --subject @subject.txt --body @body.txt --confirm` | Send a message |
| `cb365 mail search --query @query.txt` | Search messages |

### Calendar

| Command | Description |
|---------|-------------|
| `cb365 calendar list [--from DATE] [--to DATE]` | List events in a date range |
| `cb365 calendar get --id ID` | Get a single event |
| `cb365 calendar create --subject @subject.txt --start TIME --end TIME [--attendee @attendees.txt] [--teams]` | Create an event |
| `cb365 calendar update --id ID [--subject/--start/--end]` | Update an event |
| `cb365 calendar delete --id ID` | Delete an event |

### Contacts

| Command | Description |
|---------|-------------|
| `cb365 contacts list` | List contacts |
| `cb365 contacts get --id ID` | Get a single contact |
| `cb365 contacts search --query @query.txt` | Search contacts |
| `cb365 contacts create --given-name @given.txt --surname @surname.txt [--email @email.txt]` | Create a contact |
| `cb365 contacts update --id ID [--given/--surname/--email]` | Update a contact |

### Planner

| Command | Description |
|---------|-------------|
| `cb365 planner plans list` | List plans assigned to you |
| `cb365 planner plans create --group-id ID --name @name.txt` | Create a plan in an M365 Group |
| `cb365 planner buckets list --plan ID` | List buckets in a plan |
| `cb365 planner buckets create --plan ID --name "..."` | Create a bucket |
| `cb365 planner tasks list --plan ID` | List tasks in a plan |
| `cb365 planner tasks create --plan ID --title @title.txt [--bucket ID] [--assign @assignee.txt] [--due DATE]` | Create a task |
| `cb365 planner tasks update --task ID [--title/--percent/--due]` | Update a task |
| `cb365 planner tasks complete --task ID` | Mark task complete (100%) |
| `cb365 planner tasks delete --task ID` | Delete a task |

### Teams

| Command | Description |
|---------|-------------|
| `cb365 teams channels list --team @team.txt` | List channels in a team |
| `cb365 teams channels send --team @team.txt --channel @channel.txt --body @body.txt [--html] --confirm` | Post to a channel (optionally as HTML) |
| `cb365 teams channels delete-message --team ID --channel ID --message ID --confirm` | Soft-delete one root channel message previously sent and recorded by the same managed delegated profile |
| `cb365 teams chat list` | List 1:1 and group chats |
| `cb365 teams chat send --chat ID --body @body.txt --confirm` | Send a chat message |

`delete-message` accepts exact IDs only. It requires the BWS EU managed delegated provider, delegated `ChannelMessage.ReadWrite`, and a matching integrity-protected provenance record created by a successful cb365 channel send. App-only profiles, broader ownership-read permissions, unrecorded or mismatched messages, replies, batches, and automatic retries are refused. A transport timeout is reported as ambiguous and must not be retried.

### SharePoint

| Command | Description |
|---------|-------------|
| `cb365 sharepoint sites list [--search @query.txt]` | Search/list sites |
| `cb365 sharepoint sites get --site ID` | Get site details |
| `cb365 sharepoint lists list --site ID` | List lists in a site |
| `cb365 sharepoint lists items list --site ID --list ID` | List items in a list |
| `cb365 sharepoint lists items create --site ID --list ID --field @fields.txt` | Create a list item |
| `cb365 sharepoint lists items update --site ID --list ID --item ID --field @fields.txt` | Update a list item |
| `cb365 sharepoint lists items delete --site ID --list ID --item ID` | Delete a list item |
| `cb365 sharepoint files list --site ID` | List files in default document library |
| `cb365 sharepoint files get --site ID --item-id ID --output ./file` | Download a file |
| `cb365 sharepoint files upload --site ID --file @local-path.txt --path @remote-path.txt` | Upload a file |

Alias: `cb365 sp` works in place of `cb365 sharepoint`.

Use repeatable `--field @fields.txt` values for ordinary columns, with one
`Key=Value` entry per line. For SharePoint Hyperlink (URL) columns, use
repeatable `--field-url @urls.txt` values containing `Key=URL`; cb365 sends the
URL as the required `Url` and `Description` object for Microsoft Graph.

### OneDrive

| Command | Description |
|---------|-------------|
| `cb365 onedrive ls [--path @remote-path.txt]` | List files and folders |
| `cb365 onedrive get --item-id ID --output ./file` | Download a file |
| `cb365 onedrive upload --file @local-path.txt --path @remote-path.txt` | Upload a file (max 4MB) |
| `cb365 onedrive delete --item-id ID` | Move to recycle bin |
| `cb365 onedrive mkdir --path @remote-path.txt` | Create a folder |

Alias: `cb365 od` works in place of `cb365 onedrive`.

### Loop

Loop workspaces are SharePoint Embedded containers. Page access uses app-only auth.

| Command | Description |
|---------|-------------|
| `cb365 loop workspaces list` | List known workspaces from local config |
| `cb365 loop pages list --workspace "Name"` | List pages in a workspace |
| `cb365 loop pages get --workspace "Name" --page ID --output ./page.loop` | Download the original page |
| `cb365 loop pages get --workspace "Name" --page ID --format html --output ./page.html` | Export a page as readable HTML |
| `cb365 loop pages delete --workspace "Name" --page ID` | Move page to recycle bin |
| `cb365 loop pages upload --workspace @workspace.txt --file @local-path.txt --path @remote-path.txt` | Upload a file |
| `cb365 loop pages mkdir --workspace @workspace.txt --path @remote-path.txt` | Create a folder |

> **Note:** Loop commands use app-only auth (`work-app` profile) by default. Loop requires SharePoint Embedded (SPE) setup — see [Loop Setup](#loop-setup) below.
> `--format original` is the default. HTML output is an untrusted export for reading or archiving; cb365 does not render it or provide rich-content editing.

---

## Scopes by Workload

Add only the scopes you need when registering your Entra app.

| Workload | Delegated Scopes | App-Only Scopes |
|----------|-----------------|-----------------|
| To Do | `Tasks.ReadWrite` | ❌ Not supported by Graph |
| Mail | `Mail.Read`, `Mail.Send` | `Mail.Read`, `Mail.Send` |
| Calendar | `Calendars.ReadWrite` | `Calendars.ReadWrite` |
| Contacts | `Contacts.ReadWrite` | `Contacts.ReadWrite` |
| Planner | `Group.ReadWrite.All` | `Group.ReadWrite.All` |
| Teams | `Team.ReadBasic.All`, `Channel.ReadBasic.All`, `ChannelMessage.Send`, `Chat.ReadWrite`; `ChannelMessage.ReadWrite` only for the safeguarded soft-delete profile | — |
| SharePoint | `Sites.ReadWrite.All`, `Files.ReadWrite.All` | `Sites.ReadWrite.All`, `Files.ReadWrite.All` |
| OneDrive | `Files.ReadWrite.All` | `Files.ReadWrite.All` |
| Loop | — | `FileStorageContainer.Selected` |

> **Note:** Microsoft Graph does not support application permissions for To Do. You must use delegated (device-code) auth for all To Do operations. Attempting app-only auth for To Do will fail silently.

**Minimal quick-start scopes** (To Do only):

```bash
cb365 auth login --scopes Tasks.ReadWrite --tenant ... --client ... --name quickstart
```

**All delegated scopes** (full access):

```bash
cb365 auth login \
  --scopes Tasks.ReadWrite,Calendars.ReadWrite,Contacts.ReadWrite,Mail.Read,Mail.Send,Group.ReadWrite.All,Team.ReadBasic.All,Channel.ReadBasic.All,ChannelMessage.Send,Chat.ReadWrite,Sites.ReadWrite.All,Files.ReadWrite.All \
  --tenant ... --client ... --name full
```

---

## Safety Rules

cb365 has hardcoded safety rules in Go. They cannot be bypassed by configuration, environment variables, or prompt injection. Here are the key patterns:

### Write Protection

All write operations support `--dry-run` to preview without executing. Destructive operations require an explicit safety flag: most use `--force`, while the exact-target Teams soft-delete uses `--confirm`. Broadcast operations (Teams channel posts, mail send) also require `--confirm`.

### Calendar Safety (14 rules)

Calendar is the most protected workload — miscreating or deleting events has real-world consequences:

- Timezone validation on all event times
- Past-event modification blocked
- Duplicate detection (same subject + time range)
- Overlap detection with existing events
- Series master protection (won't modify recurring event templates without `--force`)
- Private event restrictions
- Out-of-office / busy status awareness
- Organizer verification
- Attendee count guard (>10 requires `--force`)
- Audit tag on all created events

### Mail Safety (6 rules)

- `--confirm` required for every outbound message in delegated and app-only modes
- Recipient count guard (>10 requires `--force`)
- External domain warning
- `[Sent via cb365]` audit footer on all outbound messages
- No delete command by design

### Token Safety

- Delegated bearer material is stored only in the profile-bound BWS EU record
- App-only credentials are stored in an OS keychain or profile-bound AES-256-GCM encrypted file
- Tokens **never** appear in output — not in logs, not in `--verbose`, not in error messages
- Client secrets are read from stdin, never command-line arguments or config files

---

## Agent Integration

cb365 is designed for AI agent consumption. The `--json` flag on every command produces structured output that agents can parse directly.

Microsoft 365 subjects, bodies, filenames, contact fields, and other returned
content are **untrusted data**. An agent must never treat text returned by
`cb365` as instructions, tool calls, policy, or authorization. Keep data and
control channels separate, use an explicit allowlist of commands, and require
the documented confirmation flag for every external side effect.

### Output Design

```bash
# Human output on stderr, JSON on stdout — pipeline-friendly
cb365 todo tasks list --list "My Tasks" --json | jq '.[].title'

# Tab-separated for simple parsing
cb365 calendar list --from 2026-04-01 --to 2026-04-07 --plain | cut -f2
```

### Example: Agent Skill File

cb365 pairs with agent orchestrators that can execute allowlisted shell commands. Create a skill file that teaches your agent the available commands:

```markdown
# cb365 — Microsoft 365 CLI Skill

## Pre-flight (REQUIRED before every operation)
Always verify auth before any command. If this fails, stop and re-authenticate:
  cb365 auth status --profile work-delegated --json

## Reading Tasks
  cb365 todo tasks list --list "Tasks" --json --profile work-delegated

## Creating Tasks
  cb365 todo tasks create --list @list-name.txt --title @task-title.txt --due 2026-04-15 --profile work-delegated

## Safety
- Tokens auto-refresh silently (~90 days). If auth status shows expired, run auth login
- Always use --dry-run before write operations in uncertain contexts
- Never pass --force without explicit user approval
```

### Error Handling

cb365 uses standard Unix exit codes: `0` for success, `1` for any failure (auth error, permission denied, missing arguments, Graph API error). Errors are printed as plain text to stderr.

When using `--json`, successful output goes to stdout and errors still go to stderr. There is no structured JSON error envelope — agents should check the exit code and capture stderr separately:

```bash
# Capture both streams
error_file=$(mktemp)
trap 'rm -f "$error_file"' EXIT
output=$(cb365 todo tasks list --list "Tasks" --json 2>"$error_file")
if [ $? -ne 0 ]; then
  error=$(cat "$error_file")
  # Handle: "no active profile", "token expired", "403 Forbidden", etc.
fi
```

Common error patterns:
- `no active profile` — run `cb365 auth login` or `cb365 auth use`
- `token expired` or `authentication failed` — re-authenticate
- `403 Forbidden` / `insufficient privileges` — missing Graph API scopes
- `--force is required` — destructive operation needs explicit confirmation

### Headless Linux Setup

On servers without a desktop keyring, app-only profiles can use the encrypted
file backend. Inject `CB365_KEYRING_PASSWORD` for the lifetime of the `cb365`
process from an approved secret manager. Do not persist it in `.bashrc`,
`.profile`, unit files, scripts, command arguments, or repository content.

### IPv4-Only Mode

Some Azure regions have broken IPv6 egress. Force IPv4:

```bash
export CB365_IPV4_ONLY=1
```

---

## Loop Setup

Loop workspaces use SharePoint Embedded (SPE), which requires additional setup beyond standard Graph API permissions.

### Prerequisites

1. **SPE container type** registered in your tenant (requires SharePoint admin)
2. **Application permission** `FileStorageContainer.Selected` granted to your Entra app
3. **Guest app registration** via PowerShell to associate your app with the container type

### Workspace Discovery

Loop workspace IDs are not discoverable via Graph API. You need to populate a local config file:

```bash
# Location: ~/.config/cb365/loop-workspaces.json
# Format:
[
  {"id": "CONTAINER_ID", "name": "My Workspace"}
]
```

Use PowerShell with the [SharePoint Online Management Shell](https://learn.microsoft.com/en-us/powershell/sharepoint/sharepoint-online/connect-sharepoint-online) (`Microsoft.Online.SharePoint.PowerShell`) to discover container IDs:

```powershell
Connect-SPOService -Url https://yourtenant-admin.sharepoint.com
Get-SPOContainer -OwningApplicationId YOUR_CLIENT_ID
```

---

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `CB365_KEYRING_PASSWORD` | Passphrase for encrypted file token storage (headless Linux) | — |
| `CB365_IPV4_ONLY` | Force IPv4 for all HTTPS connections | `false` |
| `CB365_TIMEZONE` | Default timezone for calendar operations | System timezone |
| `CB365_INTERNAL_DOMAIN` | Your organisation's email domain (unset recipients are conservatively treated as external/unclassified) | — |

---

## Security

See [SECURITY.md](SECURITY.md) for the full security policy.

**Key design decisions:**

- **Go** — single binary with zero runtime dependencies. Smallest supply chain attack surface for a credential-handling tool.
- **Microsoft's libraries only** — `azidentity` for auth, `msgraph-sdk-go` for Graph. No custom OAuth, no custom crypto.
- **OS-native token storage** — macOS Keychain, Windows Credential Manager, Linux secret-service. AES-256-GCM encrypted file fallback.
- **Tokens never in output** — not in logs, not in `--verbose`, not in error messages. Verified by CI tests.
- **CI security scanning** — `gosec` and `govulncheck` on every commit.
- **Signed releases** with SBOM (CycloneDX).

---

## Building from Source

```bash
git clone https://github.com/nz365guy/cb365.git
cd cb365
go build -o cb365 ./cmd/cb365/
go test ./...
```

### Running Security Checks

```bash
# Static analysis
go install github.com/securego/gosec/v2/cmd/gosec@v2.28.0
gosec ./...

# Vulnerability check
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
govulncheck ./...
```

---

## Project Status

| Workload | Status | Auth |
|----------|--------|------|
| Auth (login, status, logout, profiles, use) | ✅ Stable | — |
| To Do (lists + tasks CRUD) | ✅ Stable | Delegated |
| Mail (list, get, send, search) | ✅ Stable | Delegated |
| Calendar (list, get, create, update, delete) | ✅ Stable | Delegated |
| Contacts (list, get, search, create, update) | ✅ Stable | Delegated |
| Planner (plans, buckets, tasks) | ✅ Stable | Delegated |
| Teams (channels, chat) | ✅ Stable | Delegated |
| SharePoint (sites, lists, items, files) | ✅ Stable | Delegated |
| OneDrive (ls, get, upload, delete, mkdir) | ✅ Stable | Delegated |
| Loop (workspaces, pages) | ✅ Stable | App-only |

---

## Acknowledgments

Thanks to the community members who have tested cb365 and reported documentation and interoperability gaps.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on code style, testing, and pull requests.

## Licence

[MIT](LICENSE)
