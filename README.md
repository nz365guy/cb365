# cb365

Enterprise CLI for Microsoft 365 via Microsoft Graph. Entra ID authenticated. Agent-friendly.

## Overview

cb365 provides scriptable, agent-friendly access to Microsoft 365 workloads via the Microsoft Graph API. Built for enterprise environments with proper Entra ID authentication, OS keychain token storage, and structured output for automation.

## Features

- **Entra ID authentication** — device-code flow (delegated) and client credentials (app-only)
- **OS keychain storage** — tokens stored in macOS Keychain, Linux secret-service, or Windows Credential Manager
- **Agent-friendly output** — `--json` and `--plain` flags on every command
- **Multi-profile** — manage multiple tenants and auth modes simultaneously
- **Workload modules** — To Do, Planner, Mail, Calendar, Contacts, SharePoint, OneDrive

## Quick Start

```bash
# Install
go install github.com/nz365guy/cb365/cmd/cb365@latest

# Authenticate (requires an Entra ID app registration)
cb365 auth login --tenant YOUR_TENANT_ID --client YOUR_CLIENT_ID --scopes Tasks.ReadWrite

# Check status
cb365 auth status

# List your To Do task lists
cb365 todo lists --json
```

## Entra ID App Registration

1. Go to [Microsoft Entra admin center](https://entra.microsoft.com)
2. Navigate to **Identity > Applications > App registrations > New registration**
3. Name: `cb365` (or your preference)
4. Supported account types: **Single tenant**
5. Redirect URI: leave empty (device-code flow doesn't need one)
6. Under **Authentication**: enable **Allow public client flows**
7. Under **API permissions**: add Microsoft Graph delegated permissions for the workloads you need

## Security

See [SECURITY.md](SECURITY.md) for our security policy and design principles.

Tokens are stored in your operating system's native keychain — never in plaintext files.

## Status

🚧 **Pre-release** — cb365 is under active development. APIs may change. Not yet recommended for production use without your own security review.

## Licence

MIT — see [LICENSE](LICENSE)
