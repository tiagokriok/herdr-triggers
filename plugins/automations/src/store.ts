import { existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import type { RunRecord } from "./dispatch";
import { parseTasksFile, type ParseResult } from "./schema";

const LOCK_STALE_MS = 5_000;
let tmpCounter = 0;

const pause = (ms: number) => Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);

export const withLock = <T>(file: string, fn: () => T): T => {
  const lock = `${file}.lock`;
  mkdirSync(dirname(file), { recursive: true });
  const deadline = Date.now() + LOCK_STALE_MS;
  for (;;) {
    try {
      mkdirSync(lock);
      break;
    } catch (e) {
      if ((e as NodeJS.ErrnoException).code !== "EEXIST") throw e;
      if (Date.now() > deadline) rmSync(lock, { recursive: true, force: true });
      else pause(20);
    }
  }
  try {
    return fn();
  } finally {
    rmSync(lock, { recursive: true, force: true });
  }
};

export const writeJson = (file: string, data: unknown) => {
  mkdirSync(dirname(file), { recursive: true });
  const tmp = `${file}.${process.pid}.${++tmpCounter}.tmp`;
  writeFileSync(tmp, JSON.stringify(data, null, 2) + "\n");
  renameSync(tmp, file);
};

export const loadTasks = (file: string): ParseResult =>
  existsSync(file) ? parseTasksFile(readFileSync(file, "utf8")) : { success: true, data: { tasks: [] } };

export const readTasksRaw = (file: string) =>
  JSON.parse(readFileSync(file, "utf8")) as { tasks: { id: string; enabled: boolean }[] };

export const disableTasks = (file: string, ids: string[]) => {
  if (!ids.length) return;
  withLock(file, () => {
    const raw = readTasksRaw(file);
    for (const t of raw.tasks) if (ids.includes(t.id)) t.enabled = false;
    writeJson(file, raw);
  });
};

export const loadRuns = (file: string): RunRecord[] => {
  if (!existsSync(file)) return [];
  try {
    const runs = JSON.parse(readFileSync(file, "utf8")) as unknown;
    if (Array.isArray(runs)) return runs as RunRecord[];
  } catch {}
  renameSync(file, `${file}.corrupt-${Date.now()}`);
  return [];
};

export const saveRun = (file: string, run: RunRecord) =>
  withLock(file, () => {
    const runs = loadRuns(file);
    const i = runs.findIndex((r) => r.taskId === run.taskId && r.n === run.n);
    if (i >= 0) runs[i] = run;
    else runs.push(run);
    writeJson(file, runs);
  });

export const reserveRun = (file: string, run: Pick<RunRecord, "taskId" | "firedAt" | "status">): RunRecord =>
  withLock(file, () => {
    const runs = loadRuns(file);
    const n = runs.reduce((max, r) => (r.taskId === run.taskId ? Math.max(max, r.n) : max), 0) + 1;
    const record: RunRecord = { taskId: run.taskId, n, firedAt: run.firedAt, terminal: null, status: run.status };
    writeJson(file, [...runs, record]);
    return record;
  });
