import { Cron } from "croner";
import type { Schedule } from "./schema";

export const nextRun = (schedule: Schedule, after: Date): Date | null => {
  if (schedule.type === "once") {
    const runAt = new Date(schedule.runAt);
    return runAt > after ? runAt : null;
  }
  return new Cron(schedule.expr, { paused: true }).nextRun(after);
};
