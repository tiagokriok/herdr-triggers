import { agentArgv } from "./agents";
import type { Task } from "./schema";

export type ExecResult = { code: number; stdout: string; stderr: string };
export type Exec = (argv: string[]) => Promise<ExecResult>;

export type RunRecord = {
  taskId: string;
  n: number;
  firedAt: string;
  terminal: string | null;
  space?: string;
  timeoutAt?: string;
  status: string;
  error?: string;
};

export type DispatchDeps = {
  exec: Exec;
  hc: string;
  app: string;
  onUpdate: (r: RunRecord) => void;
  now: () => Date;
  pause?: (ms: number) => Promise<void>;
  configDir?: string;
};

export const ACTIVE = ["starting", "working", "blocked"];

const PROMPT_ON_SCREEN = /Enter to confirm|Do you want to proceed\?|Esc to cancel/;
const SCREEN_TAIL_LINES = 5;
const CHECK_IN = ["timeout", "message"];
const STARTUP_PAUSE_MS = 2000;

export const parseJson = <T>(text: string): T | undefined => {
  try {
    return JSON.parse(text) as T;
  } catch {
    return undefined;
  }
};

const outputOf = (r: ExecResult) => r.stderr.trim() || r.stdout.trim();

const finish = (record: RunRecord, deps: DispatchDeps, status: string, error?: string) => {
  record.status = status;
  if (error !== undefined) record.error = error;
  deps.onUpdate(record);
  return record;
};

const isBlocked = async (deps: DispatchDeps, terminal: string) => {
  const got = await deps.exec([deps.hc, "get", "--json", terminal]);
  const agentState = parseJson<{ status?: { state?: string } | null }>(got.stdout)?.status?.state;
  if (agentState === "blocked") return true;
  if (agentState === "working") return false;
  const screen = await deps.exec([deps.hc, "read", terminal]);
  return screen.code === 0 && PROMPT_ON_SCREEN.test(screen.stdout.trimEnd().split("\n").slice(-SCREEN_TAIL_LINES).join("\n"));
};

export const watchRun = async (record: RunRecord, deps: DispatchDeps): Promise<RunRecord> => {
  for (;;) {
    const waited = await deps.exec([deps.hc, "wait", "--json", "--timeout", "30", record.terminal!]);
    const result = parseJson<{ ended?: boolean; reason?: string | null; status?: string | null; turn?: number }>(waited.stdout);
    if (!result) return finish(record, deps, "failed", `hc wait: ${outputOf(waited)}`);
    const { ended, reason, status, turn } = result;
    const startupIdle = reason === "status" && status === "idle" && turn === 0;
    if (!startupIdle && (ended || (reason && !CHECK_IN.includes(reason))))
      return finish(record, deps, (reason === "status" ? status : reason) ?? "done");
    if (record.timeoutAt && deps.now() > new Date(record.timeoutAt)) {
      await deps.exec([deps.hc, "kill", record.terminal!]);
      return finish(record, deps, "timeout");
    }
    if (startupIdle) {
      await (deps.pause ?? Bun.sleep)(STARTUP_PAUSE_MS);
      continue;
    }
    const blocked = await isBlocked(deps, record.terminal!);
    if (blocked !== (record.status === "blocked")) finish(record, deps, blocked ? "blocked" : "working");
  }
};

export const runTask = async (task: Task, record: RunRecord, deps: DispatchDeps): Promise<RunRecord> => {
  const started = await deps.exec([
    deps.app, "do", "space.new",
    JSON.stringify({
      name: `${task.title} run ${record.n}`,
      folder: task.cwd,
      argv: agentArgv(task, deps.configDir),
      task: task.prompt,
    }),
  ]);
  const made = parseJson<{ space?: string; terminal?: string; error?: string }>(started.stdout);
  if (started.code !== 0 || !made?.terminal)
    return finish(record, deps, "failed", `space.new: ${made?.error ?? outputOf(started)}`);

  record.terminal = made.terminal;
  if (made.space) record.space = made.space;
  if (task.timeoutMinutes) record.timeoutAt = new Date(deps.now().getTime() + task.timeoutMinutes * 60_000).toISOString();
  finish(record, deps, "working");
  return watchRun(record, deps);
};
