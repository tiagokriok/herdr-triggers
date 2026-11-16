# herdr-triggers

Go module, one binary (`cmd/herdr-triggers`). Tests: `go test -race ./...`, `go vet ./...`.

- `internal/*`: cron/once/webhook triggers for Herdr. App backend: spaces only via documented `herdr-app do` commands (`herdr-app commands`), terminals via `hc` (`herdr-core`). CLI backend: `herdr workspace`/`agent` commands (`herdr --skill`). Never `herdr` plugins, `herdr.space`/`herdr.tab` labels or app layout files. See `docs/design.md`.
  - `task` (tasks.json schema), `schedule` (next run, tick plan), `agents` (argv per agent), `store` (paths, tasks.json edits, SQLite runs/deliveries), `config` (backend, session, public), `dispatch` (app backend: space.new + hc wait; cli backend: herdr workspace/agent), `app` (tick, reconcile, run-now), `webhook`, `tunnel` (public URL providers), `service` (systemd/launchd), `update` (daily release check), `panel` (embedded panel/skill install), `daemon` (start/stop/status), `view` (`list --json`), `tui` (Bubble Tea + Huh), `runtime` (real exec, env).
  - Tests inject `Exec`/`Now`; set `time.Local` explicitly in time-dependent tests.
- `assets.go` (root package): embeds `mods/triggers` and `skills/schedule-local` into the binary.
- `mods/triggers`: Herdr app panel mod (gpui-kit JS, no DOM/Node/setTimeout; types in `~/.config/herdr-app/*.d.ts`). Talks only to the binary via `exec` (`~/.config/herdr-triggers/cli.json`). Check with `herdr-app check`. `RECIPE.md` + `preview.png` for herdr.dev/mods.
- `skills/schedule-local`: agent skill editing `~/.config/herdr-triggers/tasks.json`.
- Deps: gronx (cron), modernc sqlite, bubbletea/huh/lipgloss. Add others only with a reason. No code comments. Conventional commits. Public repo: keep user-specific paths and names out.

## Agent skills

### Issue tracker

GitHub Issues on `tiagokriok/herdr-triggers` via `gh`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` and `docs/adr/` at the root, created lazily. See `docs/agents/domain.md`.
