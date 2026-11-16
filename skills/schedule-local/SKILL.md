---
name: schedule-local
description: Schedule local agent runs with herdr-triggers (cron, once, webhook) on this machine or server. Use to add, change, pause, resume, remove or run-now a task, or to wire a webhook from GitHub, Linear, Grafana and others.
---

# schedule-local

`herdr-triggers` runs each task on its trigger: it opens a Herdr app space or a `herdr` CLI workspace in the task's `cwd`, starts the agent there, and sends the prompt as the first message. Tasks live in one JSON file, which the scheduler re-reads every 5 s. `herdr-triggers help` lists every command.

## Steps

1. **Gather.** Ask only for what is missing: the prompt, the `cwd` (absolute), and the trigger. Derive the `id` and `title` yourself. Done when every required field below has a value.
2. **Edit.** The file is `${XDG_CONFIG_HOME:-~/.config}/herdr-triggers/tasks.json`. Create `{ "tasks": [] }` if it is missing. Change only the entry you are working on:
   - pause or resume: flip `enabled`;
   - remove: delete the entry.
   Done when the file holds exactly the change asked for.
3. **Validate.** Run `herdr-triggers list`. It fails loudly on bad JSON, a bad cron or an unknown field. Done when it exits 0 and the user has confirmed the next fire times it prints.
4. **Webhook only.**
   - Generate the token with `herdr-triggers token`.
   - Run `herdr-triggers public`. If it shows provider `none`, ask which tunnel to use (`tailscale`, `ngrok [domain]`, `cloudflared`, `url <baseUrl>`), set it with `herdr-triggers public <provider>`, then run `herdr-triggers service restart`.
   - Give the user the full URL `<public url>/hooks/<id>/<token>`. If the sender signs requests, also give them the signing secret to paste.

   Done when the user has a URL that works from outside.
5. **Scheduler.** Run `herdr-triggers status`. If it says `stopped`, run `herdr-triggers service install`; on a server, also run `sudo loginctl enable-linger $USER`. Done when it says `live`.
6. **Run now** (when asked): `herdr-triggers run-now <id> --bg`.

## Task fields

```json
{ "id": "daily-review", "title": "Daily review", "prompt": "review open PRs and summarize",
  "cwd": "/abs/path/to/repo", "schedule": { "type": "cron", "expr": "0 9 * * 1-5" },
  "enabled": true, "agent": "claude" }
```

- Required:
  - `id` matches `^[a-z0-9][a-z0-9-]*$` and is unique;
  - `title`;
  - `prompt`;
  - `cwd` (absolute);
  - `schedule`;
  - `enabled`.
- `schedule` is one of:
  - `{ "type": "cron", "expr": "<5 fields, local time>" }`
  - `{ "type": "once", "runAt": "<ISO 8601 with offset>" }`. It disables itself after firing.
  - `{ "type": "webhook", "token": "<herdr-triggers token>", "signature": {…} }`. The `signature` is optional (see below).
- Optional fields:
  - `agent`: `claude` (default), `codex`, `pi`, `opencode`, or any binary, which then gets `--model <m>` and the prompt as its last argument;
  - `model`;
  - `agentArgs`: flags passed verbatim before the prompt;
  - `timeoutMinutes`;
  - `catchUpMinutes` (default 120): how late a run that was due while the machine slept may still fire;
  - `claudeConfigDir`.
- Unknown fields are rejected.

## Webhook signature

Add `signature` when the sender signs its requests. The body is then checked with HMAC-SHA256 on top of the token.

| sender | `signature` |
|---|---|
| GitHub | `{ "secret": "<s>", "header": "X-Hub-Signature-256", "encoding": "hex", "prefix": "sha256=" }` |
| Linear | `{ "secret": "<s>", "header": "Linear-Signature", "encoding": "hex" }` |
| other | `header` is the sender's header; `encoding` is `hex` (default) or `base64`; `prefix` is optional |

- The `secret` has 16+ characters and must be the same value set in the sender.
- A wrong token, a bad signature and an unknown task all answer `404`.
- The real reason shows in the TUI's delivery log.

## Runs

- Statuses:
  - `starting`, `working`: the agent is running.
  - `blocked`: the agent is waiting for input.
  - `done`, `error`, `exit`, `failed`: the run ended.
  - `timeout`: the run passed `timeoutMinutes`.
  - `missed`: the run was due too long ago to fire.
- A task never overlaps itself.
- `failed`, `blocked`, `timeout` and `missed` raise a desktop notification.
- History keeps the last 50 runs and deliveries per task; change it with `herdr-triggers keep <n>` (`0` keeps everything).
- The user can do all of this in `herdr-triggers tui`.
