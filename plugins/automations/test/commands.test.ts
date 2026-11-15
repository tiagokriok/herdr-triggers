import { beforeEach, describe, expect, test } from "bun:test";
import { existsSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describeSchedule, listTasks, setEnabled } from "../src/commands";

let files: { tasks: string; runs: string };

beforeEach(() => {
  const dir = mkdtempSync(join(tmpdir(), "cmd-"));
  files = { tasks: join(dir, "tasks.json"), runs: join(dir, "runs.json") };
});

const base = { prompt: "p", cwd: "/tmp", enabled: true };
const daily = { ...base, id: "daily", title: "Daily", schedule: { type: "cron", expr: "0 9 * * *" } };
const paused = { ...base, id: "paused", title: "Paused", enabled: false, schedule: { type: "cron", expr: "30 18 * * 1-5" } };

describe("describeSchedule", () => {
  test.each([
    [{ type: "cron", expr: "0 9 * * *" }, "daily at 09:00"],
    [{ type: "cron", expr: "30 18 * * 1-5" }, "weekdays at 18:30"],
    [{ type: "cron", expr: "0 8 * * 1" }, "Mondays at 08:00"],
    [{ type: "cron", expr: "0 0 1 * *" }, "monthly on day 1 at 00:00"],
    [{ type: "cron", expr: "15 * * * *" }, "hourly at :15"],
    [{ type: "cron", expr: "*/10 * * * *" }, "every 10 min"],
    [{ type: "cron", expr: "0 9 1-7 * 1" }, "cron 0 9 1-7 * 1"],
    [{ type: "once", runAt: "2026-11-14T10:05:00Z" }, "once on 2026-11-14 10:05"],
  ] as const)("%j -> %s", (schedule, text) => {
    expect(describeSchedule(schedule)).toBe(text);
  });
});

describe("listTasks", () => {
  test("each task with its schedule text, next run and last run", () => {
    writeFileSync(files.tasks, JSON.stringify({ tasks: [daily, paused] }));
    writeFileSync(files.runs, JSON.stringify([
      { taskId: "daily", n: 1, firedAt: "2026-11-13T09:00:00.000Z", terminal: "t1", status: "done", space: "sp1" },
      { taskId: "daily", n: 2, firedAt: "2026-11-14T09:00:00.000Z", terminal: "t2", status: "blocked", space: "sp2" },
    ]));
    expect(listTasks(files, new Date("2026-11-14T10:00:00Z"))).toEqual({
      tasks: [
        {
          id: "daily", title: "Daily", schedule: "daily at 09:00", enabled: true,
          next: "2026-11-15T09:00:00.000Z",
          last: { n: 2, status: "blocked", firedAt: "2026-11-14T09:00:00.000Z", space: "sp2", terminal: "t2" },
        },
        { id: "paused", title: "Paused", schedule: "weekdays at 18:30", enabled: false, next: null, last: null },
      ],
    });
  });

  test("invalid tasks file reports its error with no tasks", () => {
    writeFileSync(files.tasks, "{bad");
    const r = listTasks(files, new Date());
    expect(r.tasks).toEqual([]);
    expect(r.error).toContain("invalid json");
  });
});

describe("setEnabled", () => {
  test("flips one task and keeps the others", () => {
    writeFileSync(files.tasks, JSON.stringify({ tasks: [daily, paused] }));
    setEnabled(files.tasks, "paused", true);
    setEnabled(files.tasks, "daily", false);
    const saved = JSON.parse(readFileSync(files.tasks, "utf8")).tasks;
    expect(saved.map((t: { enabled: boolean }) => t.enabled)).toEqual([false, true]);
    expect(saved[1].schedule).toEqual(paused.schedule);
    expect(existsSync(`${files.tasks}.lock`)).toBe(false);
  });

  test("unknown id throws", () => {
    writeFileSync(files.tasks, JSON.stringify({ tasks: [daily] }));
    expect(() => setEnabled(files.tasks, "nope", true)).toThrow("unknown task: nope");
  });
});
