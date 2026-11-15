import { join } from "node:path";

type Env = Record<string, string | undefined>;

export const paths = (env: Env = process.env) => {
  if (!env.HOME) throw new Error("HOME is not set");
  const config = env.XDG_CONFIG_HOME || join(env.HOME, ".config");
  const state = join(env.XDG_STATE_HOME || join(env.HOME, ".local", "state"), "automations");
  return {
    tasks: join(config, "automations", "tasks.json"),
    cli: join(config, "automations", "cli.json"),
    runs: join(state, "runs.json"),
    scheduler: join(state, "scheduler.json"),
  };
};
