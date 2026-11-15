import { beforeEach, describe, expect, test } from "bun:test";
import { existsSync, mkdtempSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { paths } from "../src/paths";
import { disableTasks, loadRuns, loadTasks, reserveRun, saveRun, withLock } from "../src/store";

describe("paths", () => {
  test("honours XDG dirs", () => {
    expect(paths({ XDG_CONFIG_HOME: "/c", XDG_STATE_HOME: "/s", HOME: "/h" })).toEqual({
      tasks: "/c/automations/tasks.json",
      cli: "/c/automations/cli.json",
      runs: "/s/automations/runs.json",
      scheduler: "/s/automations/scheduler.json",
    });
  });

  test("falls back to HOME", () => {
    expect(paths({ HOME: "/h" })).toEqual({
      tasks: "/h/.config/automations/tasks.json",
      cli: "/h/.config/automations/cli.json",
      runs: "/h/.local/state/automations/runs.json",
      scheduler: "/h/.local/state/automations/scheduler.json",
    });
  });

  test("throws without HOME", () => {
    expect(() => paths({ XDG_CONFIG_HOME: "/c" })).toThrow("HOME");
  });
});

describe("store", () => {
  let dir: string;
  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "auto-"));
  });

  const task = (id: string, enabled = true) => ({
    id, title: id, prompt: "p", cwd: "/tmp", schedule: { type: "once", runAt: "2026-11-14T10:00:00Z" }, enabled,
  });

  test("missing tasks file is an empty list", () => {
    expect(loadTasks(join(dir, "nope", "tasks.json"))).toEqual({ success: true, data: { tasks: [] } });
  });

  test("disableTasks flips only the given ids and keeps the rest of the file", () => {
    const file = join(dir, "tasks.json");
    writeFileSync(file, JSON.stringify({ tasks: [task("a"), task("b")] }));
    disableTasks(file, ["a"]);
    const saved = JSON.parse(readFileSync(file, "utf8"));
    expect(saved.tasks.map((t: { enabled: boolean }) => t.enabled)).toEqual([false, true]);
    expect(saved.tasks[0].schedule).toEqual(task("a").schedule);
  });

  test("saveRun appends new runs and updates existing ones by task+n", () => {
    const file = join(dir, "state", "runs.json");
    expect(loadRuns(file)).toEqual([]);
    const r = { taskId: "a", n: 1, firedAt: "2026-11-14T10:00:00.000Z", terminal: "t1", status: "working" };
    saveRun(file, r);
    saveRun(file, { ...r, status: "done" });
    saveRun(file, { ...r, n: 2 });
    expect(loadRuns(file).map((x) => [x.n, x.status])).toEqual([[1, "done"], [2, "working"]]);
  });

  test("corrupt runs.json is moved aside and read as empty", () => {
    const file = join(dir, "runs.json");
    writeFileSync(file, "{oops");
    expect(loadRuns(file)).toEqual([]);
    expect(existsSync(file)).toBe(false);
    expect(readdirSync(dir).some((f) => f.startsWith("runs.json.corrupt-"))).toBe(true);
  });

  test("reserveRun saves a starting record with the next n per task", () => {
    const file = join(dir, "runs.json");
    const a1 = reserveRun(file, { taskId: "a", firedAt: "x", status: "starting" });
    const b1 = reserveRun(file, { taskId: "b", firedAt: "x", status: "starting" });
    const a2 = reserveRun(file, { taskId: "a", firedAt: "y", status: "missed" });
    expect([a1.n, b1.n, a2.n]).toEqual([1, 1, 2]);
    expect(loadRuns(file)).toEqual([
      { taskId: "a", n: 1, firedAt: "x", terminal: null, status: "starting" },
      { taskId: "b", n: 1, firedAt: "x", terminal: null, status: "starting" },
      { taskId: "a", n: 2, firedAt: "y", terminal: null, status: "missed" },
    ]);
  });

  test("withLock is exclusive and releases on throw", () => {
    const file = join(dir, "runs.json");
    expect(() => withLock(file, () => { throw new Error("x"); })).toThrow("x");
    expect(existsSync(`${file}.lock`)).toBe(false);
    expect(withLock(file, () => existsSync(`${file}.lock`))).toBe(true);
  });

  test("writes leave no tmp files behind", () => {
    const file = join(dir, "runs.json");
    saveRun(file, { taskId: "a", n: 1, firedAt: "x", terminal: null, status: "done" });
    expect(readdirSync(dir)).toEqual(["runs.json"]);
  });
});
