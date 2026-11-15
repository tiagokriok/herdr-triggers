# herdr-triggers

**Cron, one-off and webhook triggers for any coding agent in [Herdr](https://herdr.dev): the desktop app, the `herdr` CLI, your laptop, a VPS or a homelab.**

A Grafana alert, a ClickUp task or a cron line opens a new space (app) or workspace (CLI) with Claude Code, Codex, Pi or OpenCode in your repo, with the prompt (and the webhook body) as its first message. One Go binary that runs as a user service, a JSON task file, a TUI to edit it and a side panel in the app.

![Triggers panel](mods/triggers/preview.png)

- **Triggers**: 5-field cron (local time, `L`/`W`/`#` supported), once at a date, or a webhook with a per-task token and a public URL (Tailscale Funnel, ngrok, Cloudflare Tunnel or your own).
- **Any agent**: `claude`, `codex`, `pi`, `opencode` know their model flag and prompt argument; any other binary gets `--model <m>` and the prompt last. Per task `model`, `agentArgs`, `timeoutMinutes`.
- **Survives sleep**: a run due while the machine slept fires on wake within `catchUpMinutes` (default 120); older ones are recorded `missed`.
- **Never overlaps itself**: a fire or webhook is refused while the task's previous run is still working or blocked.
- **App or CLI**: runs open in the Herdr app (opened for you if it is closed) or in the headless `herdr` server (started for you if it is down), so it works on a server with no desktop.
- **Runs in the background**: `herdr-triggers service install` runs the scheduler and the tunnel as a systemd (Linux) or launchd (macOS) user service.
- **Tells you when it matters**: `failed`, `blocked`, `error`, `timeout` and `missed` raise a desktop notification.
- **Webhook payloads are untrusted**: the body reaches the agent framed between random-tagged markers, labelled as data, not instructions. Bodies over 64 KB are refused. Every delivery is logged.

## Install

Requires Go 1.26+ and Herdr: the desktop app (`herdr-app`, `herdr-core`/`hc`) or the CLI (`herdr`), or both.

```sh
git clone https://github.com/tiagokriok/herdr-triggers && cd herdr-triggers
go build -trimpath -ldflags="-s -w" -o ~/.local/bin/herdr-triggers ./cmd/herdr-triggers
herdr-triggers service install             # scheduler + webhooks + tunnel, started on login
```

Desktop app panel (optional):

```sh
cp -r mods/triggers ~/.config/herdr-app/mods/
herdr-app check ~/.config/herdr-app/mods/triggers
```

`service install` writes a systemd user unit (`~/.config/systemd/user/herdr-triggers.service`, logs with `journalctl --user -u herdr-triggers`) or a launchd agent (`~/Library/LaunchAgents/dev.herdr.triggers.plist`). It copies your `PATH` and `CLAUDE_CONFIG_DIR` into it, so reinstall after changing them. `service restart` applies config changes; `service uninstall` removes it. It also writes `~/.config/herdr-triggers/cli.json`, which the panel reads to find the binary.

Without the service, `herdr-triggers start` runs the scheduler in a "Triggers" space of the app instead (`stop` closes it); it stops with that space.

### Where runs open

`herdr-triggers backend [auto|app|cli] [session]`, then `service restart`:

- `app`: a new space in the Herdr app (`herdr-app do space.new`). If no window is open, the app is launched first.
- `cli`: a new workspace in the `herdr` server (`herdr workspace create`, `herdr agent start`). If the server is down, `herdr server` is started headless. Attach later with `herdr` or `herdr --remote <host>`. An optional session name keeps runs in their own `herdr --session <name>`. The agent must be a `herdr agent start --kind`; newlines in the prompt reach it as `\n`.
- `auto` (default): `app` when `herdr-app` is on `PATH`, else `cli`.

### On a VPS or homelab

```sh
go build -trimpath -ldflags="-s -w" -o ~/.local/bin/herdr-triggers ./cmd/herdr-triggers
herdr-triggers backend cli
herdr-triggers public tailscale            # or ngrok <domain>, url <baseUrl>
herdr-triggers service install
sudo loginctl enable-linger $USER          # keep user services running without a login
```

Log in to your agents once on the server (`claude`, `codex login`, ...) so a run never stops at a login screen.

Optional skill for agents (`/schedule-local`): `ln -sfn "$PWD/skills/schedule-local" ~/.claude/skills/schedule-local`.

## Use

```sh
herdr-triggers tui              # browse, add, edit, enable/disable, run now, delete
herdr-triggers list [--json]
herdr-triggers run-now <id> [--bg]
herdr-triggers enable <id> | disable <id>
herdr-triggers status [--json]  # live/stopped, service or space, backend
herdr-triggers service install | uninstall | restart
herdr-triggers backend [auto|app|cli] [session]
herdr-triggers start | stop     # scheduler in an app space instead of a service
herdr-triggers token            # a random webhook token
herdr-triggers public [provider [x]]  # how webhooks reach you (see below)
```

- Tasks: `~/.config/herdr-triggers/tasks.json` (format in `skills/schedule-local/SKILL.md`), re-read every 5 s. Edit it by hand, with the TUI, or let an agent use the skill.
- Config: `~/.config/herdr-triggers/config.json`: `{"backend": "auto", "session": "", "public": {"provider": "none"}}`.
- State: `~/.local/state/herdr-triggers/state.db` (SQLite: runs and webhook deliveries).
- Env: `TRIGGERS_ADDR` (default `127.0.0.1:8787`), `TRIGGERS_HC`, `TRIGGERS_APP`, `TRIGGERS_HERDR`, `CLAUDE_CONFIG_DIR` (forwarded to claude runs).

### Webhooks

```json
{ "id": "grafana-alert", "title": "Investigate alert", "prompt": "Find the cause of this alert and propose a fix.",
  "cwd": "/abs/path/to/repo", "schedule": { "type": "webhook", "token": "<herdr-triggers token>" },
  "enabled": true, "agent": "claude" }
```

```sh
curl -X POST https://<public url>/hooks/grafana-alert/<token> -d '{"alert":"cpu"}'
```

The token can be the last path segment (one URL to paste in Grafana, ClickUp, GitHub), or go in `Authorization: Bearer`, `X-Triggers-Token` or `?token=`. Replies: `202 {"task","run"}`, `401` bad token, `404` no enabled webhook task, `409` previous run still active, `413` body over 64 KB. In the TUI, `c` copies a task's full URL.

#### Public URL

The server listens on `127.0.0.1:8787`. Choose how it is reached from outside with `herdr-triggers public <provider>` or `p` in the TUI, then restart the scheduler (`s` in the TUI, or `service restart`). The scheduler runs the tunnel itself and stops it on exit.

| provider | runs | URL |
|---|---|---|
| `none` (default) | nothing | `http://127.0.0.1:8787` |
| `tailscale` | `tailscale funnel 8787` (enable Funnel on your tailnet first) | `https://<machine>.<tailnet>.ts.net` |
| `ngrok [domain]` | `ngrok http` (with `--url` for a reserved domain) | random, or your domain |
| `cloudflared` | quick tunnel | random `*.trycloudflare.com` |
| `url <baseUrl>` | nothing: you run the tunnel or proxy (named Cloudflare Tunnel, nginx, ...) | `<baseUrl>` |

Random URLs (ngrok without a domain, cloudflared) change on every scheduler restart; use `tailscale`, an ngrok domain or `url` for senders you configure once. `herdr-triggers public` prints the current URL or why the tunnel failed.

### What this does on your machine

- The scheduler runs as a user service (or in a Herdr app space) and runs `herdr-app` and `hc`, or `herdr`, each task's agent, and `notify-send` (`osascript` on macOS) for alerts. It may launch the app or `herdr server` when they are not running.
- It writes `~/.config/herdr-triggers/` (only when you or the TUI change tasks or config, and `cli.json` on `start`/`service install`), `~/.local/state/herdr-triggers/`, and the service unit on `service install`.
- It opens one HTTP port, on `127.0.0.1` only, and, if you choose a provider, starts `tailscale funnel`, `ngrok` or `cloudflared` to expose it.
- The **mod runs programs** with `exec`: the `herdr-triggers` binary named in `~/.config/herdr-triggers/cli.json`, which it reads from outside its own folder. It changes `tasks.json` only when you press Enable/Disable. It does not use the network.

## Similar projects

- [DnzzL/herdr-automations](https://github.com/DnzzL/herdr-automations): a mature cron scheduler for Herdr, TUI and app, with a git worktree per run and YAML config. Choose it for worktree-isolated runs on a clock; choose this one for webhook triggers, one-off runs and per-agent argv. Some ideas here (catch-up window, failure alerts, per-task model) were inspired by it.
- [T3 Code](https://github.com/pingdotgg/t3code) scheduled tasks inspired the SQLite run/delivery log and the framing of untrusted webhook payloads.

## Dev

`go test -race ./...`, `go vet ./...`. Design notes: `docs/design.md`. Mod brief: `mods/triggers/RECIPE.md`.

## License

MIT
