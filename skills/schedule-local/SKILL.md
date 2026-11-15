---
name: schedule-local
description: Use when the user wants to schedule, list, pause, resume, remove or run-now a local recurring/one-off agent automation (Claude Code, Codex, Pi, OpenCode, ...) on this machine (Herdr desktop app), like /schedule but local.
---

# schedule-local

Local automations run by `herdr-triggers` inside the Herdr desktop app, on cron, once or webhook triggers. Each run opens a new app space named `<title> run <n>` with the agent in the task's `cwd`, and the prompt as its first message.

## Files
- Tasks: `${XDG_CONFIG_HOME:-~/.config}/herdr-triggers/tasks.json` (re-read every 5s)
- Runs and webhook deliveries: `${XDG_STATE_HOME:-~/.local/state}/herdr-triggers/state.db` (SQLite; read them with `herdr-triggers list --json` or the TUI)
- Binary: `herdr-triggers` (on PATH, usually `~/.local/bin`)

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
- `schedule`: `{ "type": "cron", "expr": "<exactly 5 fields, local time>" }` or `{ "type": "once", "runAt": "<ISO with offset, e.g. 2026-11-15T09:00:00-03:00>" }` or `{ "type": "webhook", "token": "<herdr-triggers token>" }` (fires on `POST <public url>/hooks/<id>/<token>`; `herdr-triggers public` shows the URL; the body is appended to the prompt as untrusted data).
- Unknown fields are rejected.
- `agent` optional, default `claude`. Known: `claude`, `codex`, `pi`, `opencode`; any other binary gets `--model <m>` and the prompt as last argument. `once` tasks auto-disable after firing.
- `model` optional: passed as the agent's model flag (`--model`, `-m` for codex).
- `agentArgs` optional: extra flags, verbatim, before the prompt.
- `timeoutMinutes` optional: past it the run is killed and recorded `timeout`.
- `catchUpMinutes` optional, default 120: a run due while the machine slept fires on wake if at most this late; later ones are `missed`. `0` = only within 60s.
- `claudeConfigDir` optional, claude only: used when the scheduler has no `CLAUDE_CONFIG_DIR` in its env (`herdr-triggers start` forwards the caller's).
- A missed `once` task gets a `missed` run and is disabled.
- Failed, blocked, timed-out and missed runs raise a desktop notification (`notify-send` / macOS `osascript`).
- A task never overlaps itself: a fire is skipped while its previous run is still working/blocked.
- Run status: `starting`, `working`, `blocked` (agent waits for input or a confirm prompt is on screen), `done`, `error`, `exit`, `missed`, `failed`, `timeout`.

## Flow
1. Ask only what is missing: what to do (prompt), where (cwd, absolute), when. Derive `id`/`title`.
2. Read the file (create `{ "tasks": [] }` if missing). Edit with minimal JSON changes; never touch other tasks.
   - pause/resume: flip `enabled`. remove: delete the entry.
3. Validate and show next fire times: `herdr-triggers list`.
   It fails loudly on invalid JSON/cron. Confirm next times with the user. For a webhook, generate the token with `herdr-triggers token` and give the user the full URL. If `public` says provider `none`, ask which tunnel they want (`herdr-triggers public tailscale|ngrok [domain]|cloudflared|url <baseUrl>`), then restart with `herdr-triggers stop && herdr-triggers start`.
4. Make sure the scheduler runs: `herdr-triggers start` (idempotent; needs the Herdr app open).
5. Run now: `herdr-triggers run-now <id> --bg`. pause/resume also: `herdr-triggers disable|enable <id>`.
6. The user can also do all of this in `herdr-triggers tui`.
