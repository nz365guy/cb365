# cb365

**Scriptable access to Microsoft 365 from the command line.**

If you need to automate Microsoft 365 — create tasks, send mail, manage calendars, post to Teams, work with SharePoint, OneDrive, or Planner — you currently have two options: write a custom app against the Graph API, or click through the UI by hand. cb365 gives you a third: a single command-line tool that does it all, with structured output that scripts and AI agents can consume directly.

```bash
# List your tasks as JSON
cb365 todo tasks list --list "My Tasks" --json

# Create a calendar event with a Teams link
cb365 calendar create --subject "Design Review" --start "2026-04-10T10:00:00+12:00" \
  --end "2026-04-10T10:30:00+12:00" --attendee "colleague@example.com" --teams

# Send mail (with safety confirmation)
cb365 mail send --to "team@example.com" --subject "Update" --body "Shipped v2." --confirm
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

### Authenticate (2 minutes)

```bash
# Delegated auth (interactive — you sign in via browser)
cb365 auth login \
  --tenant YOUR_TENANT_ID \
  --client YOUR_CLIENT_ID \
  --scopes Tasks.ReadWrite \
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
cb365 todo tasks create --list "My Tasks" --title "Try cb365" --due 2026-04-15

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
  --name work
```

The token expires after approximately 1 hour. Re-run the login command to refresh.

### App-Only (Client Secret)

For unattended automation. The app authenticates with a client secret. Requires application permissions (not delegated) in Entra.

```bash
cb365 auth login \
  --mode app-only \
  --tenant TENANT_ID \
  --client CLIENT_ID \
  --client-secret YOUR_SECRET \
  --name automation
```

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
| `cb365 todo lists create --name "..."` | Create a task list |
| `cb365 todo lists update --list ID --name "..."` | Rename a task list |
| `cb365 todo lists delete --list ID` | Delete a task list |
| `cb365 todo tasks list --list "Name"` | List tasks (accepts name or ID) |
| `cb365 todo tasks get --list X --task Y` | Get a single task |
| `cb365 todo tasks create --list X --title "..." [--body "..."] [--due YYYY-MM-DD]` | Create a task |
| `cb365 todo tasks update --list X --task Y [--title/--status/--body/--due]` | Update a task |
| `cb365 todo tasks complete --list X --task Y` | Mark task complete |
| `cb365 todo tasks delete --list X --task Y` | Delete a task |

### Mail

| Command | Description |
|---------|-------------|
| `cb365 mail list` | List inbox messages |
| `cb365 mail get --id ID` | Get a single message |
| `cb365 mail send --to addr --subject "..." --body "..." --confirm` | Send a message |
| `cb365 mail search --query "keyword"` | Search messages |

### Calendar

| Command | Description |
|---------|-------------|
| `cb365 calendar list [--from DATE] [--to DATE]` | List events in a date range |
| `cb365 calendar get --id ID` | Get a single event |
| `cb365 calendar create --subject "..." --start TIME --end TIME [--attendee email] [--teams]` | Create an event |
| `cb365 calendar update --id ID [--subject/--start/--end]` | Update an event |
| `cb365 calendar delete --id ID` | Delete an event |

### Contacts

| Command | Description |
|---------|-------------|
| `cb365 contacts list` | List contacts |
| `cb365 contacts get --id ID` | Get a single contact |
| `cb365 contacts search --query "name"` | Search contacts |
| `cb365 contacts create --given "..." --surname "..." [--email "..."]` | Create a contact |
| `cb365 contacts update --id ID [--given/--surname/--email]` | Update a contact |

### Planner

| Command | Description |
|---------|-------------|
| `cb365 planner plans list` | List plans assigned to you |
| `cb365 planner plans create --group ID --title "..."` | Create a plan in an M365 Group |
| `cb365 planner buckets list --plan ID` | List buckets in a plan |
| `cb365 planner buckets create --plan ID --name "..."` | Create a bucket |
| `cb365 planner tasks list --plan ID` | List tasks in a plan |
| `cb365 planner tasks create --plan ID --title "..." [--bucket ID] [--assign email] [--due DATE]` | Create a task |
| `cb365 planner tasks update --task ID [--title/--percent/--due]` | Update a task |
| `cb365 planner tasks complete --task ID` | Mark task complete (100%) |
| `cb365 planner tasks delete --task ID` | Delete a task |

### Teams

| Command | Description |
|---------|-------------|
| `cb365 teams channels list --team "Name"` | List channels in a team |
| `cb365 teams channels send --team "Name" --channel "General" --body "..." --confirm` | Post to a channel |
| `cb365 teams chat list` | List 1:1 and group chats |
| `cb365 teams chat send --chat ID --body "..."` | Send a chat message |

### SharePoint

| Command | Description |
|---------|-------------|
| `cb365 sharepoint sites list [--search "..."]` | Search/list sites |
| `cb365 sharepoint sites get --site ID` | Get site details |
| `cb365 sharepoint lists list --site ID` | List lists in a site |
| `cb365 sharepoint lists items list --site ID --list ID` | List items in a list |
| `cb365 sharepoint lists items create --site ID --list ID --fields '{...}'` | Create a list item |
| `cb365 sharepoint lists items update --site ID --list ID --item ID --fields '{...}'` | Update a list item |
| `cb365 sharepoint lists items delete --site ID --list ID --item ID` | Delete a list item |
| `cb365 sharepoint files list --site ID` | List files in default document library |
| `cb365 sharepoint files get --site ID --item-id ID --output ./file` | Download a file |
| `cb365 sharepoint files upload --site ID --file ./doc --path "/folder/doc"` | Upload a file |

Alias: `cb365 sp` works in place of `cb365 sharepoint`.

### OneDrive

| Command | Description |
|---------|-------------|
| `cb365 onedrive ls [--path /folder]` | List files and folders |
| `cb365 onedrive get --item-id ID --output ./file` | Download a file |
| `cb365 onedrive upload --file ./doc --path "/Documents/doc"` | Upload a file (max 4MB) |
| `cb365 onedrive delete --item-id ID` | Move to recycle bin |
| `cb365 onedrive mkdir --path "/New Folder"` | Create a folder |

Alias: `cb365 od` works in place of `cb365 onedrive`.

### Loop

Loop workspaces are SharePoint Embedded containers. Page access uses app-only auth.

| Command | Description |
|---------|-------------|
| `cb365 loop workspaces list` | List known workspaces from local config |
| `cb365 loop pages list --workspace "Name"` | List pages in a workspace |
| `cb365 loop pages get --workspace "Name" --item-id ID --output ./page` | Download a page |
| `cb365 loop pages delete --workspace "Name" --item-id ID` | Move page to recycle bin |
| `cb365 loop pages upload --workspace "Name" --file ./doc --path "/folder/doc"` | Upload a file |
| `cb365 loop pages mkdir --workspace "Name" --path "/New Folder"` | Create a folder |

> **Note:** Loop commands use app-only auth (`work-app` profile) by default. Loop requires SharePoint Embedded (SPE) setup — see [Loop Setup](#loop-setup) below.

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
| Teams | `Team.ReadBasic.All`, `Channel.ReadBasic.All`, `ChannelMessage.Send`, `Chat.ReadWrite` | — |
| SharePoint | `Sites.ReadWrite.All`, `Files.ReadWrite.All` | `Sites.ReadWrite.All`, `Files.ReadWrite.All` |
| OneDrive | `Files.ReadWrite.All` | `Files.ReadWrite.All` |
| Loop | — | `FileStorageContainer.Selected` |

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

cb365 has 44 safety rules hardcoded in Go. They cannot be bypassed by configuration, environment variables, or prompt injection. Here are the key patterns:

### Write Protection

All write operations support `--dry-run` to preview without executing. Destructive operations (delete, overwrite) require `--force`. Broadcast operations (Teams channel posts, mail send) require `--confirm`.

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

- `--confirm` required in delegated mode
- Recipient count guard (>10 requires `--force`)
- External domain warning
- `[Sent via cb365]` audit footer on all outbound messages
- No delete command by design

### Token Safety

- Tokens are **never** stored in plaintext — OS keychain or AES-256-GCM encrypted file only
- Tokens **never** appear in output — not in logs, not in `--verbose`, not in error messages
- Client secrets stored encrypted in keychain, never in config files

---

## Agent Integration

cb365 is designed for AI agent consumption. The `--json` flag on every command produces structured output that agents can parse directly.

### Output Design

```bash
# Human output on stderr, JSON on stdout — pipeline-friendly
cb365 todo tasks list --list "My Tasks" --json | jq '.[].title'

# Tab-separated for simple parsing
cb365 calendar list --from 2026-04-01 --to 2026-04-07 --plain | cut -f2
```

### Example: Agent Skill File

cb365 pairs well with AI agent orchestrators like OpenCLAW, LangChain, AutoGen, or any framework that can execute shell commands. Create a skill file that teaches your agent the available commands:

```markdown
# cb365 — Microsoft 365 CLI Skill

## Authentication
The agent uses a pre-configured profile. Check status before operations:
  cb365 auth status --profile work --json

## Reading Tasks
  cb365 todo tasks list --list "Tasks" --json --profile work

## Creating Tasks
  cb365 todo tasks create --list "Tasks" --title "Review PR" --due 2026-04-15 --profile work

## Safety
- Always use --dry-run before write operations in uncertain contexts
- Never pass --force without explicit user approval
- Check auth status before any operation
```

### Headless Linux Setup

On servers without a desktop keyring (common for agent VMs), set a keyring password:

```bash
export CB365_KEYRING_PASSWORD="your-secure-passphrase"
```

cb365 will use AES-256-GCM encrypted file storage instead of the OS keychain. Add this to your shell profile (`.bashrc`, `.profile`) for persistence.

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

Use PowerShell with the SharePoint Online module to discover container IDs:

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
| `CB365_INTERNAL_DOMAIN` | Your organisation's email domain (for external recipient warnings) | — |

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
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...

# Vulnerability check
go install golang.org/x/vuln/cmd/govulncheck@latest
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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on code style, testing, and pull requests.

## Licence

[MIT](LICENSE)

