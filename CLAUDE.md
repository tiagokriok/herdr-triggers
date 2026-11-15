# herdr-automations

Bun monorepo (`plugins/*` workspaces). Runtime + tests: Bun (`bun test`, `bun run typecheck`).

- `plugins/automations`: local scheduler for the Herdr **desktop app**. Spaces only via documented `herdr-app do` commands (`herdr-app commands`); terminals via `hc` (`herdr-core`). Never the `herdr` CLI/plugins, never `herdr.space`/`herdr.tab` labels or app layout files. See `docs/design.md`.
  - Logic: `schema.ts` (zod), `agents.ts` (argv per agent), `schedule.ts`, `tick.ts`, `dispatch.ts`, `app.ts`, `store.ts`, `daemon.ts`. Entrypoints: `scheduler.ts`, `cli.ts` (`commands.ts` backs `list --json`/`enable`/`disable`), `runtime.ts`, `bin/start|stop`.
  - Tests inject `exec`/`now`; TZ pinned to UTC in `test/setup.ts`.
- `mods/automations`: Herdr app panel mod (gpui-kit JS, no DOM/Node/setTimeout; types in `~/.config/herdr-app/*.d.ts`). Talks only to the CLI via `exec`. Check with `herdr-app check`. `RECIPE.md` + `preview.png` for herdr.dev/mods.
- `skills/schedule-local`: Claude skill editing `~/.config/automations/tasks.json`.
- Deps: only `croner`, `zod`. No code comments. Conventional commits. Public repo: keep user-specific paths and names out.
