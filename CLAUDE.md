# herdr-triggers

Go module, one binary (`cmd/herdr-triggers`). Tests: `go test -race ./...`, `go vet ./...`.

- `internal/*`: cron/once/webhook triggers for the Herdr **desktop app**. Spaces only via documented `herdr-app do` commands (`herdr-app commands`); terminals via `hc` (`herdr-core`). Never the `herdr` CLI/plugins, never `herdr.space`/`herdr.tab` labels or app layout files. See `docs/design.md`.
  - `task` (tasks.json schema), `schedule` (next run, tick plan), `agents` (argv per agent), `store` (paths, tasks.json edits, SQLite runs/deliveries), `dispatch` (space.new + hc wait), `app` (tick, reconcile, run-now), `webhook`, `daemon` (start/stop/status), `view` (`list --json`), `tui` (Bubble Tea + Huh), `runtime` (real exec, env).
  - Tests inject `Exec`/`Now`; set `time.Local` explicitly in time-dependent tests.
- `mods/triggers`: Herdr app panel mod (gpui-kit JS, no DOM/Node/setTimeout; types in `~/.config/herdr-app/*.d.ts`). Talks only to the binary via `exec` (`~/.config/herdr-triggers/cli.json`). Check with `herdr-app check`. `RECIPE.md` + `preview.png` for herdr.dev/mods.
- `skills/schedule-local`: agent skill editing `~/.config/herdr-triggers/tasks.json`.
- Deps: gronx (cron), modernc sqlite, bubbletea/huh/lipgloss. Add others only with a reason. No code comments. Conventional commits. Public repo: keep user-specific paths and names out.
