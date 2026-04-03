---
name: cb365
description: Microsoft 365 integration via cb365 CLI — Tasks, Mail, Calendar, Contacts, Planner, Teams, SharePoint, OneDrive, Loop
---

# cb365 — Microsoft 365 CLI Skill

Enterprise CLI for Microsoft 365 via Microsoft Graph API.
Agent-consumable output via `--json` flag on every command.

## Prerequisites

- cb365 binary installed and on PATH
- At least one authenticated profile (`cb365 auth status`)
- Entra ID app registration with required scopes

## Output Flags

| Flag | Purpose |
|------|---------|
| `--json` | Structured JSON to stdout (always use for agent parsing) |
| `--plain` | Tab-separated values to stdout |
| `--dry-run` | Preview write operations without executing |
| `--force` | Required for destructive operations (delete, overwrite) |
| `--confirm` | Required for broadcast operations (mail send, Teams channel post) |
| `--profile NAME` | Override active profile for this command |

## Auth Check

Before any operation, verify authentication:

```bash
cb365 auth status --json
```

If expired, the agent should report "auth expired" and stop — do not attempt re-auth automatically.

---

## Tasks (Microsoft To Do)

Requires `Tasks.ReadWrite` scope. Delegated auth only.

```bash
# List all task lists
cb365 todo lists list --json

# List tasks in a specific list (name or ID)
cb365 todo tasks list --list "My Tasks" --json

# Create a task
cb365 todo tasks create --list "My Tasks" --title "Review PR #42" --due 2026-04-15

# Complete a task
cb365 todo tasks complete --list "My Tasks" --task TASK_ID

# Delete (requires --force in non-interactive mode)
cb365 todo tasks delete --list "My Tasks" --task TASK_ID --force
```

---

## Mail (Outlook)

Requires `Mail.Read` and `Mail.Send` scopes.

```bash
# List recent messages
cb365 mail list --json

# Get a specific message
cb365 mail get MESSAGE_ID --json

# Search messages
cb365 mail search --query "quarterly report" --json

# Send mail (requires --confirm in delegated mode)
cb365 mail send --to "user@example.com" --subject "Subject" --body "Body" --confirm

# Preview before sending
cb365 mail send --to "user@example.com" --subject "Subject" --body "Body" --dry-run
```

**Safety:** All outbound mail includes `[Sent via cb365]` audit footer. Sending to >10 recipients requires `--force`.

---

## Calendar

Requires `Calendars.ReadWrite` scope.

### Critical Calendar Safety Rules

1. **Always include timezone offset** in `--start` and `--end` values (RFC3339 format):
   ```bash
   # CORRECT
   --start "2026-04-10T09:00:00+12:00"

   # WRONG — never use bare datetimes
   --start "2026-04-10T09:00:00"
   ```

2. **Verify dates before creating** — never trust mental arithmetic:
   ```bash
   date -d '2026-04-10' +%A  # Confirm day of week
   ```

3. **Never modify past events** — check current time before any write operation.

4. **Check for duplicates** before creating:
   ```bash
   cb365 calendar list --from 2026-04-10 --to 2026-04-11 --json
   ```

5. **Recurring events** — modify single instances only, never the series master.

### Commands

```bash
# List events in date range
cb365 calendar list --from 2026-04-10 --to 2026-04-17 --json

# Get event details
cb365 calendar get EVENT_ID --json

# Create event with attendee and Teams link
cb365 calendar create \
  --subject "Design Review" \
  --start "2026-04-10T10:00:00+12:00" \
  --end "2026-04-10T10:30:00+12:00" \
  --attendee "colleague@example.com" \
  --teams

# Update event
cb365 calendar update EVENT_ID --subject "Updated Title"

# Delete (use --dry-run first)
cb365 calendar delete EVENT_ID --dry-run
cb365 calendar delete EVENT_ID
```

---

## Contacts

Requires `Contacts.ReadWrite` scope.

```bash
cb365 contacts list --json
cb365 contacts get CONTACT_ID --json
cb365 contacts search --query "Jane" --json
cb365 contacts create --given "Jane" --surname "Doe" --email "jane@example.com"
cb365 contacts update CONTACT_ID --email "new@example.com"
```

---

## Planner

Requires `Group.ReadWrite.All` scope (admin consent).

```bash
# List plans assigned to user
cb365 planner plans list --json

# List buckets in a plan
cb365 planner buckets list --plan PLAN_ID --json

# List tasks
cb365 planner tasks list --plan PLAN_ID --json

# Create task with assignment and due date
cb365 planner tasks create --plan PLAN_ID --bucket BUCKET_ID \
  --title "Review document" --assign user@example.com --due 2026-04-15

# Update progress: not-started, in-progress, complete
cb365 planner tasks update --task TASK_ID --progress in-progress

# Complete
cb365 planner tasks complete --task TASK_ID

# Delete (requires --force)
cb365 planner tasks delete --task TASK_ID --force
```

**ETag handling:** The CLI fetches the task first to get the ETag automatically — no manual ETag management needed.

---

## Teams

Requires `Team.ReadBasic.All`, `Channel.ReadBasic.All`, `ChannelMessage.Send`, `Chat.ReadWrite` scopes.

```bash
# List channels (team name or ID)
cb365 teams channels list --team "Engineering" --json

# Post to channel (requires --confirm)
cb365 teams channels send --team "Engineering" --channel "General" \
  --body "Build passed — deploying to staging" --confirm

# List chats
cb365 teams chat list --json

# Send chat message
cb365 teams chat send --chat CHAT_ID --body "Quick update on the PR"
```

**Safety:** Channel posts include `[Sent via cb365]` audit footer and require `--confirm`.

---

## SharePoint

Requires `Sites.ReadWrite.All`, `Files.ReadWrite.All` scopes.

```bash
# Search sites
cb365 sharepoint sites list --search "Intranet" --json

# Get site details
cb365 sharepoint sites get --site SITE_ID --json

# List SharePoint lists
cb365 sharepoint lists list --site SITE_ID --json

# List items in a list
cb365 sharepoint lists items list --site SITE_ID --list LIST_ID --json

# CRUD on list items
cb365 sharepoint lists items create --site SITE_ID --list LIST_ID --fields '{"Title":"New Item"}'
cb365 sharepoint lists items update --site SITE_ID --list LIST_ID --item ITEM_ID --fields '{"Status":"Done"}'
cb365 sharepoint lists items delete --site SITE_ID --list LIST_ID --item ITEM_ID --force

# Document library files
cb365 sharepoint files list --site SITE_ID --json
cb365 sharepoint files get --site SITE_ID --item-id ITEM_ID --output ./download.pdf
cb365 sharepoint files upload --site SITE_ID --file ./report.pdf --path "/Documents/report.pdf"
```

Alias: `cb365 sp` works in place of `cb365 sharepoint`.

---

## OneDrive

Requires `Files.ReadWrite.All` scope.

```bash
cb365 onedrive ls --json                                          # Root
cb365 onedrive ls --path "/Documents" --json                      # Subfolder
cb365 onedrive get --item-id ITEM_ID --output ./report.pdf        # Download
cb365 onedrive upload --file ./data.csv --path "/Uploads/data.csv" # Upload (max 4MB)
cb365 onedrive mkdir --path "/New Folder"                          # Create folder
cb365 onedrive delete --item-id ITEM_ID --force                    # Recycle bin
```

Alias: `cb365 od` works in place of `cb365 onedrive`.

**Safety:** Upload validates file size before starting. `--force` required for overwrite and delete.

---

## Loop

Requires `FileStorageContainer.Selected` scope. App-only auth only.

```bash
# List known workspaces
cb365 loop workspaces list --json

# List pages in a workspace (name or container ID)
cb365 loop pages list --workspace "Team Notes" --json

# Download a page
cb365 loop pages get --workspace "Team Notes" --item-id ITEM_ID --output ./page.loop

# Upload a file
cb365 loop pages upload --workspace "Team Notes" --file ./doc.md --path "/docs/doc.md"

# Create folder
cb365 loop pages mkdir --workspace "Team Notes" --path "/New Section"

# Delete (recycle bin)
cb365 loop pages delete --workspace "Team Notes" --item-id ITEM_ID --force
```

**Note:** Loop commands automatically use the app-only profile. Loop requires SharePoint Embedded (SPE) setup — see the README for prerequisites.

---

## Error Handling

- **Auth expired:** `cb365 auth status` returns non-zero. Report to user, do not attempt re-auth.
- **Permission denied:** Check Entra scopes. Some workloads need admin consent.
- **Rate limited:** Graph API returns 429. Wait and retry with backoff.
- **Not found:** Verify IDs. Use list commands first to discover valid IDs.

## Agent Best Practices

1. Always check `cb365 auth status --json` before operations
2. Use `--dry-run` before any write operation in uncertain contexts
3. Never pass `--force` without explicit user approval
4. Parse `--json` output — never scrape human-readable table output
5. List before get — discover valid IDs from list commands
6. Verify calendar dates before creating — use system `date` command

