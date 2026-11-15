import { nextRun } from "./schedule";
import type { Task } from "./schema";

export type TickState = Record<string, { sig: string; at: string | null }>;

export type Missed = { task: Task; at: string };

export type TickPlan = { fire: Task[]; missed: Missed[]; state: TickState };

export const GRACE_MS = 60_000;

const signature = (task: Task) => JSON.stringify(task.schedule);

const windowMs = (task: Task) => Math.max(GRACE_MS, task.catchUpMinutes * 60_000);

const iso = (d: Date | null) => (d ? d.toISOString() : null);

export const planTick = (tasks: Task[], prev: TickState, now: Date): TickPlan => {
  const fire: Task[] = [];
  const missed: Missed[] = [];
  const state: TickState = {};

  for (const task of tasks) {
    if (!task.enabled) continue;
    const sig = signature(task);
    const armed = prev[task.id];

    if (!armed || armed.sig !== sig) {
      if (task.schedule.type === "once" && new Date(task.schedule.runAt) <= now) {
        const runAt = new Date(task.schedule.runAt);
        if (now.getTime() - runAt.getTime() <= windowMs(task)) fire.push(task);
        else missed.push({ task, at: runAt.toISOString() });
      }
      state[task.id] = { sig, at: iso(nextRun(task.schedule, now)) };
      continue;
    }

    if (armed.at === null || new Date(armed.at) > now) {
      state[task.id] = armed;
      continue;
    }

    if (now.getTime() - new Date(armed.at).getTime() <= windowMs(task)) fire.push(task);
    else missed.push({ task, at: armed.at });
    state[task.id] = { sig, at: iso(nextRun(task.schedule, now)) };
  }

  return { fire, missed, state };
};
