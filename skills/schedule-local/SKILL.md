---
name: schedule-local
description: Use when the user wants to schedule, list, pause, resume, remove or run-now a local recurring/one-off Claude automation on this machine (Herdr desktop app), like /schedule but local.
---

# schedule-local

Local scheduled automations run by the `automations` scheduler inside the Herdr desktop app. Each run opens a new app space named `<title> run <n>` with the agent in the task's `cwd`, and the prompt as its first message.

## Files
- Tasks: `${XDG_CONFIG_HOME:-~/.config}/automations/tasks.json` (re-read every 5s)
- Runs: `${XDG_STATE_HOME:-~/.local/state}/automations/runs.json` (read-only for you)
- Plugin dir: `<repo>/plugins/automations` (`<repo>` = where herdr-automations is cloned; resolve it via the symlink of this skill)

## tasks.json shape
```json
{ "tasks": [ {
  "id": "daily-review",
  "title": "Daily review",
  "prompt": "review open PRs and summarize",
  "cwd": "/abs/path/to/repo",
  "schedule": { "type": "cron", "expr": "0 9 * * 1-5" },
  "enabled": true,
  "agent": "claude"
} ] }
```
- `id`: `^[a-z0-9][a-z0-9-]*$`, unique. `cwd`: absolute path.
- `schedule`: `{ "type": "cron", "expr": "<exactly 5 fields, local time>" }` or `{ "type": "once", "runAt": "<ISO with offset, e.g. 2026-11-15T09:00:00-03:00>" }`.
- `agent` optional, default `claude`. `once` tasks auto-disable after firing.
- `claudeConfigDir` optional: used only when the scheduler has no `CLAUDE_CONFIG_DIR` in its env (`bin/start` forwards the caller's).
- Missed runs (scheduler down) are skipped, not replayed. A missed `once` task gets a `missed` run and is disabled.
- A task never overlaps itself: a fire is skipped while its previous run is still working/blocked.
- Run status in runs.json: `starting`, `working`, `blocked` (agent waits for input or a confirm prompt is on screen), `done`, `error`, `exit`, `missed`, `failed`.

## Flow
1. Ask only what is missing: what to do (prompt), where (cwd, absolute), when. Derive `id`/`title`.
2. Read the file (create `{ "tasks": [] }` if missing). Edit with minimal JSON changes; never touch other tasks.
   - pause/resume: flip `enabled`. remove: delete the entry.
3. Validate and show next fire times: `cd <repo>/plugins/automations && bun src/cli.ts list`.
   It fails loudly on invalid JSON/cron. Confirm next times with the user.
4. Make sure the scheduler runs: `bin/start` (idempotent; needs the Herdr app open).
5. Run now: `bun src/cli.ts run-now <id>` (blocks until the run ends; Ctrl-C is safe, the scheduler picks the run up again on restart).
