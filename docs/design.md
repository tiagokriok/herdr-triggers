# herdr-triggers design

One Go binary, `cmd/herdr-triggers`. Packages in `internal/`:

- **task**: `tasks.json` schema and validation (unknown fields rejected, defaults `agent: claude`, `catchUpMinutes: 120`). Cron validated by gronx, exactly 5 fields.
- **schedule**: `Next` (cron in local time, once, webhook never) and `Plan` (tick: fire, missed, armed state). A run late by at most `max(60s, catchUpMinutes)` fires, so a run due during sleep fires on wake; older ones are `missed`. A once task already past when first seen fires within that window, else is `missed`.
- **agents**: argv per agent: model flag (`--model`, `-m` for codex), `agentArgs`, prompt separator (`--`, `--prompt` for opencode, nothing for unknown binaries). `CLAUDE_CONFIG_DIR` via `env` for claude only.
- **store**: paths (XDG), `tasks.json` edits under `flock` with atomic writes, SQLite `state.db` (modernc, WAL, busy timeout, immediate transactions) for `runs` and `deliveries`. `Reserve` assigns run `n` per task in one transaction.
- **dispatch**: `herdr-app do space.new {name, folder, argv, task}`, then `hc wait --timeout 30` loop. `message` and `timeout` reasons are check-ins; `idle` at turn 0 is startup, not the end (pause 2 s). On check-ins: `timeoutAt` passed → `hc kill`, `timeout`; `blocked` from the agent status or a confirm prompt in the screen's last 5 lines while not working.
- **app**: `Step` (tick), `Reconcile` (active runs: gone terminal → `exit`, live → watched again), `Begin`/`RunNow` (overlap guard, reserve, disable once tasks, payload framing). Alerts on `failed`/`blocked`/`timeout`/`error`/`missed`.
- **webhook**: `POST /hooks/{id}`, token from Bearer, `X-Triggers-Token` or `?token=`, compared in constant time. Body ≤ 64 KB. Each delivery logged with its outcome (`accepted`, `unauthorized`, `too_large`, `busy`).
- **daemon**: `start` writes `cli.json`, reuses the remembered `{space, terminal}` while it runs `herdr-triggers serve`, else closes it and opens a "Triggers" space. `stop`, `status`.
- **view**: `list --json` for the mod and the TUI. **tui**: Bubble Tea + Huh.
- Never write app layout files or `herdr.space`/`herdr.tab` labels; only documented `herdr-app do` commands and `hc`.
- **Mod**: `mods/triggers` reads `~/.config/herdr-triggers/cli.json` and polls `list --json` / `status --json` every 10 s; actions call `run-now --bg`, `enable`, `disable`, `start`. See its `RECIPE.md`.
