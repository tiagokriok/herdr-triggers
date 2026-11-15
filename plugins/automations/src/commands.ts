import { nextRun } from "./schedule";
import type { Schedule } from "./schema";
import { loadRuns, loadTasks, readTasksRaw, withLock, writeJson } from "./store";

const DAYS = ["Sundays", "Mondays", "Tuesdays", "Wednesdays", "Thursdays", "Fridays", "Saturdays", "Sundays"];
const pad = (n: number | string) => String(n).padStart(2, "0");
const isNum = (s: string) => /^\d+$/.test(s);

export const describeSchedule = (schedule: Schedule): string => {
  if (schedule.type === "once") {
    const d = new Date(schedule.runAt);
    return `once on ${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }
  const [min, hour, dom, mon, dow] = schedule.expr.trim().split(/\s+/) as [string, string, string, string, string];
  const every = /^\*\/(\d+)$/.exec(min);
  if (every && hour === "*" && dom === "*" && mon === "*" && dow === "*") return `every ${every[1]} min`;
  if (isNum(min) && hour === "*" && dom === "*" && mon === "*" && dow === "*") return `hourly at :${pad(min)}`;
  if (!isNum(min) || !isNum(hour) || mon !== "*") return `cron ${schedule.expr}`;
  const at = `at ${pad(hour)}:${pad(min)}`;
  if (dom === "*" && dow === "*") return `daily ${at}`;
  if (dom === "*" && dow === "1-5") return `weekdays ${at}`;
  if (dom === "*" && isNum(dow) && Number(dow) <= 7) return `${DAYS[Number(dow)]} ${at}`;
  if (isNum(dom) && dow === "*") return `monthly on day ${Number(dom)} ${at}`;
  return `cron ${schedule.expr}`;
};

export type TaskView = {
  id: string;
  title: string;
  schedule: string;
  enabled: boolean;
  next: string | null;
  last: { n: number; status: string; firedAt: string; space: string | null; terminal: string | null } | null;
};

export const listTasks = (files: { tasks: string; runs: string }, now: Date): { tasks: TaskView[]; error?: string } => {
  const loaded = loadTasks(files.tasks);
  if (!loaded.success) return { tasks: [], error: loaded.error };
  const runs = loadRuns(files.runs);
  return {
    tasks: loaded.data.tasks.map((t) => {
      const last = runs.filter((r) => r.taskId === t.id).at(-1);
      return {
        id: t.id,
        title: t.title,
        schedule: describeSchedule(t.schedule),
        enabled: t.enabled,
        next: t.enabled ? (nextRun(t.schedule, now)?.toISOString() ?? null) : null,
        last: last
          ? { n: last.n, status: last.status, firedAt: last.firedAt, space: last.space ?? null, terminal: last.terminal }
          : null,
      };
    }),
  };
};

export const setEnabled = (file: string, id: string, enabled: boolean) =>
  withLock(file, () => {
    const raw = readTasksRaw(file);
    const task = raw.tasks.find((t) => t.id === id);
    if (!task) throw new Error(`unknown task: ${id}`);
    task.enabled = enabled;
    writeJson(file, raw);
  });
