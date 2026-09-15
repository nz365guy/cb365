# Decommissioned

The OpenClaw DevOps team and software factory (dev-team agents, the
approval-queue pipeline, and their supporting units/timers/config) were
decommissioned on vm-openclaw-01 on 14-15 September 2026.

This repo (`cb365`) was barely touched by that machinery. Its full commit
history (`git log --oneline`) is ordinary feature, fix, dependency-bump, and
CI work — no dev-team, factory, or approval-queue commits — and it has no
entries in `~/openclaw-decommission/STATUS.md` or `FINDINGS.md`. There is
nothing to archive here; no `archive/decommission-2026-09/` folder exists
in this repo.

Do not rebuild any of it. Software here, as everywhere else in the
OpenCLAW estate, is now built interactively with Claude Code or Codex —
see `docs/how-software-gets-built.md` in `openclaw-config`.
