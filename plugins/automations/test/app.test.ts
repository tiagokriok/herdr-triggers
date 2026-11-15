import { beforeEach, describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { reconcile, runNow, step, type AppDeps } from "../src/app";
import type { Exec } from "../src/dispatch";
import { loadRuns } from "../src/store";

let dir: string;
let files: AppDeps["files"];
let calls: string[][];
let logs: string[];
let liveTerminals: Set<string>;
let alerts: string[];
let waitResult: unknown;

const exec: Exec = async (argv) => {
  calls.push(argv);
  const out = (o: unknown) => ({ code: 0, stdout: JSON.stringify(o), stderr: "" });
  if (argv[0] === "herdr-app") return out({ space: "sp5", tab: "tb5", terminal: "t5" });
  if (argv[1] === "get")
    return liveTerminals.has(argv.at(-1)!) ? out({ state: "live", status: null }) : { code: 1, stdout: "", stderr: "not_found" };
  return out(waitResult);
};

const writeTasks = (tasks: unknown[]) => writeFileSync(files.tasks, JSON.stringify({ tasks }));
const readTasks = () => JSON.parse(readFileSync(files.tasks, "utf8")).tasks as { id: string; enabled: boolean }[];
const writeRuns = (runs: unknown[]) => writeFileSync(files.runs, JSON.stringify(runs));
const once = { id: "hi", title: "Hi", prompt: "say hi", cwd: "/tmp", schedule: { type: "once", runAt: "2026-11-14T10:00:00Z" }, enabled: true };
const cron = { ...once, id: "daily", title: "Daily", schedule: { type: "cron", expr: "0 9 * * *" } };

beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "app-"));
  files = { tasks: join(dir, "tasks.json"), runs: join(dir, "runs.json") };
  calls = [];
  logs = [];
  liveTerminals = new Set();
  alerts = [];
  waitResult = { ended: true, reason: "status", status: "done" };
});

const deps = (now: string): AppDeps => ({
  files, exec, hc: "hc", app: "herdr-app", now: () => new Date(now), log: (m) => logs.push(m),
  notify: (title, body) => alerts.push(`${title}|${body}`),
});

describe("notify", () => {
  test("a failed run raises one alert naming the task and run", async () => {
    writeTasks([cron]);
    waitResult = { ended: true, reason: "status", status: "failed" };
    await runNow("daily", deps("2026-11-14T09:00:00Z"));
    expect(alerts).toEqual(["Automations: daily #1 failed|"]);
  });

  test("an agent error raises an alert", async () => {
    writeTasks([cron]);
    waitResult = { ended: true, reason: "status", status: "error" };
    await runNow("daily", deps("2026-11-14T09:00:00Z"));
    expect(alerts).toEqual(["Automations: daily #1 error|"]);
  });

  test("a done run stays quiet", async () => {
    writeTasks([cron]);
    await runNow("daily", deps("2026-11-14T09:00:00Z"));
    expect(alerts).toEqual([]);
  });

  test("a missed once-task raises an alert", async () => {
    writeTasks([once]);
    await step({}, deps("2026-11-14T13:00:00Z"));
    expect(alerts).toEqual(["Automations: hi missed|due 2026-11-14T10:00:00.000Z"]);
  });
});

describe("step", () => {
  test("due once-task dispatches, records the run and disables itself", async () => {
    writeTasks([once]);
    const armed = await step({}, deps("2026-11-14T09:59:00Z"));
    expect(calls).toEqual([]);
    const fired = await step(armed.state, deps("2026-11-14T10:00:03Z"));
    await Promise.all(fired.runs);
    expect(calls.map((c) => c[0] === "herdr-app" ? "space.new" : c[1])).toEqual(["space.new", "wait"]);
    expect(readTasks()[0]!.enabled).toBe(false);
    expect(loadRuns(files.runs)).toMatchObject([{ taskId: "hi", n: 1, terminal: "t5", space: "sp5", status: "done" }]);
  });

  test("once-task is disabled only after its run record is saved", async () => {
    writeTasks([once]);
    const armed = await step({}, deps("2026-11-14T09:59:00Z"));
    let enabledAtSpaceNew: boolean | undefined;
    let runsAtSpaceNew = 0;
    const spy: Exec = async (argv) => {
      if (argv[0] === "herdr-app") {
        enabledAtSpaceNew = readTasks()[0]!.enabled;
        runsAtSpaceNew = loadRuns(files.runs).length;
      }
      return exec(argv);
    };
    await Promise.all((await step(armed.state, { ...deps("2026-11-14T10:00:03Z"), exec: spy })).runs);
    expect(runsAtSpaceNew).toBe(1);
    expect(enabledAtSpaceNew).toBe(false);
  });

  test("invalid tasks file keeps previous state and fires nothing", async () => {
    writeFileSync(files.tasks, "{bad");
    const prev = { hi: { sig: "x", at: null } };
    expect((await step(prev, deps("2026-11-14T10:00:00Z"))).state).toBe(prev);
    expect(calls).toEqual([]);
  });

  test("missed cron run is logged, not replayed", async () => {
    writeTasks([cron]);
    const armed = await step({}, deps("2026-11-14T08:00:00Z"));
    const r = await step(armed.state, deps("2026-11-15T12:00:00Z"));
    expect(r.runs).toEqual([]);
    expect(logs).toContain("missed daily 2026-11-14T09:00:00.000Z");
  });

  test("once-task past due with no run is recorded missed and disabled", async () => {
    writeTasks([once]);
    await step({}, deps("2026-11-14T13:00:00Z"));
    expect(calls).toEqual([]);
    expect(loadRuns(files.runs)).toEqual([
      { taskId: "hi", n: 1, firedAt: "2026-11-14T10:00:00.000Z", terminal: null, status: "missed" },
    ]);
    expect(readTasks()[0]!.enabled).toBe(false);
    expect(logs).toContain("missed hi 2026-11-14T10:00:00.000Z");
  });

  test("once-task past due that already ran is only disabled", async () => {
    writeTasks([once]);
    writeRuns([{ taskId: "hi", n: 1, firedAt: "2026-11-14T10:00:00.000Z", terminal: "t1", status: "done" }]);
    await step({}, deps("2026-11-14T13:00:00Z"));
    expect(loadRuns(files.runs)).toHaveLength(1);
    expect(readTasks()[0]!.enabled).toBe(false);
  });

  test("overlap guard skips a fire while the previous run is still working", async () => {
    writeTasks([cron]);
    writeRuns([{ taskId: "daily", n: 1, firedAt: "x", terminal: "t1", status: "working" }]);
    const armed = await step({}, deps("2026-11-14T08:59:00Z"));
    await Promise.all((await step(armed.state, deps("2026-11-14T09:00:02Z"))).runs);
    expect(calls).toEqual([]);
    expect(loadRuns(files.runs)).toHaveLength(1);
    expect(logs.some((l) => l.startsWith("skip daily"))).toBe(true);
  });
});

describe("reconcile", () => {
  test("active runs whose terminal is gone become exit; live ones are watched", async () => {
    liveTerminals.add("t2");
    writeRuns([
      { taskId: "a", n: 1, firedAt: "x", terminal: "t1", status: "working" },
      { taskId: "b", n: 1, firedAt: "x", terminal: "t2", status: "blocked" },
      { taskId: "c", n: 1, firedAt: "x", terminal: null, status: "starting" },
      { taskId: "d", n: 1, firedAt: "x", terminal: "t4", status: "done" },
    ]);
    await Promise.all(await reconcile(deps("2026-11-14T12:00:00Z")));
    expect(loadRuns(files.runs).map((r) => [r.taskId, r.status])).toEqual([
      ["a", "exit"], ["b", "done"], ["c", "exit"], ["d", "done"],
    ]);
    expect(calls.filter((c) => c[1] === "wait").map((c) => c.at(-1))).toEqual(["t2"]);
  });
});

describe("runNow", () => {
  test("dispatches a task immediately with the next run number", async () => {
    writeTasks([{ ...once, enabled: false }]);
    writeRuns([{ taskId: "hi", n: 4, firedAt: "x", terminal: "t1", status: "done" }]);
    const r = await runNow("hi", deps("2026-11-14T12:00:00Z"));
    expect(r).toMatchObject({ n: 5, status: "done" });
    expect(JSON.parse(calls[0]![3]!).name).toBe("Hi run 5");
  });

  test("unknown id throws", async () => {
    writeTasks([once]);
    expect(runNow("nope", deps("2026-11-14T12:00:00Z"))).rejects.toThrow("unknown task");
  });
});
