import { existsSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { envArgv, parseJson, type Exec } from "./dispatch";
import { writeJson } from "./store";

export type DaemonDeps = {
  exec: Exec;
  hc: string;
  app: string;
  file: string;
  cliFile: string;
  bun: string;
  pluginDir: string;
  configDir?: string;
};

type SchedulerRef = { space: string; terminal: string };

const SCHEDULER_ENTRY = "src/scheduler.ts";

const remembered = (file: string) =>
  existsSync(file) ? parseJson<SchedulerRef>(readFileSync(file, "utf8")) : undefined;

const runsScheduler = async (deps: DaemonDeps, terminal: string) => {
  const got = await deps.exec([deps.hc, "get", "--json", terminal]);
  const t = got.code === 0 ? parseJson<{ state?: string; argv?: string[]; foreground?: { argv?: string[] } }>(got.stdout) : undefined;
  return t?.state === "live" && (t.foreground?.argv ?? []).some((a) => a.endsWith(SCHEDULER_ENTRY));
};

const closeSpace = (deps: DaemonDeps, space: string) =>
  deps.exec([deps.app, "do", "space.close", "--id", space, "--confirmed", "true"]);

export const startScheduler = async (deps: DaemonDeps): Promise<string> => {
  writeJson(deps.cliFile, { argv: [deps.bun, join(deps.pluginDir, "src", "cli.ts")] });
  const prev = remembered(deps.file);
  if (prev && (await runsScheduler(deps, prev.terminal))) return `already running ${prev.terminal}`;
  if (prev) await closeSpace(deps, prev.space);

  const made = await deps.exec([
    deps.app, "do", "space.new",
    JSON.stringify({ name: "Automations", folder: deps.pluginDir, argv: envArgv(deps.configDir, ["bun", SCHEDULER_ENTRY]) }),
  ]);
  const ref = parseJson<Partial<SchedulerRef> & { error?: string }>(made.stdout);
  if (made.code !== 0 || !ref?.space || !ref.terminal)
    throw new Error(`space.new failed: ${ref?.error ?? (made.stderr.trim() || made.stdout.trim())}`);
  writeJson(deps.file, { space: ref.space, terminal: ref.terminal });
  return `started ${ref.terminal}`;
};

export const stopScheduler = async (deps: DaemonDeps): Promise<string> => {
  const prev = remembered(deps.file);
  if (!prev) return "not running";
  await closeSpace(deps, prev.space);
  rmSync(deps.file, { force: true });
  return "stopped";
};

export const schedulerStatus = async (
  deps: DaemonDeps,
): Promise<{ state: "live"; space: string; terminal: string } | { state: "stopped" }> => {
  const prev = remembered(deps.file);
  return prev && (await runsScheduler(deps, prev.terminal))
    ? { state: "live", space: prev.space, terminal: prev.terminal }
    : { state: "stopped" };
};
