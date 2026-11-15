# automations design

- **Scheduler**: `bun src/scheduler.ts` runs in an app space made by `herdr-app do space.new` (`bin/start`). App terminals are core-kept (`keep: true`). `bin/start` remembers `{space, terminal}` in the state dir and does nothing while that terminal still runs the scheduler; otherwise it closes the old space and opens a new one.
- **Tick** (5s): re-read `tasks.json`, `planTick` decides fire/missed. Cron is 5 fields, local time (croner). Runs within 60s of their time fire; older ones are logged `missed` and skipped. A missed `once` task gets a `missed` run and is disabled.
- **Dispatch**: reserve run `n` in `runs.json` (lockfile, atomic writes), disable `once` tasks, then `herdr-app do space.new {name, folder, argv: [env CLAUDE_CONFIG_DIR=…, agent, --], task: prompt}`. Track with `hc wait --timeout 30`; on each check-in, `blocked` comes from the agent status, or a confirm prompt in the screen's last lines while the agent isn't working.
- **Overlap**: a fire is skipped while the task's last run is starting/working/blocked.
- **Restart**: on start, active runs whose terminal is gone become `exit`; live ones are watched again.
- Never write app layout files or `herdr.space`/`herdr.tab` labels; only documented `herdr-app do` commands and `hc`.
- **Mod**: `mods/automations` reads `~/.config/automations/cli.json` (written by `start`) and polls `list --json` / `status --json` every 10s; actions call `run-now --bg`, `enable`, `disable`, `start`. See its `RECIPE.md`.
