import { describe, expect, test } from "bun:test";
import { runTask, watchRun, type DispatchDeps, type Exec, type RunRecord } from "../src/dispatch";
import type { Task } from "../src/schema";

const task: Task = {
  id: "nightly",
  title: "Nightly triage",
  prompt: "triage the inbox",
  cwd: "/home/me/proj",
  schedule: { type: "cron", expr: "0 2 * * *" },
  enabled: true,
  agent: "claude",
  catchUpMinutes: 120,
};

const ok = (o: unknown) => ({ code: 0, stdout: JSON.stringify(o), stderr: "" });
const made = { space: "sp1", tab: "tb1", terminal: "t9" };
const blockedGet = { state: "live", status: { app: "claude-code", state: "blocked" } };

type Fake = { waits?: unknown[]; gets?: unknown[]; screens?: string[]; spaceNew?: { code: number; stdout: string; stderr: string } };

const fakeExec = ({ waits = [], gets = [], screens = [], spaceNew = ok(made) }: Fake = {}) => {
  const calls: string[][] = [];
  const exec: Exec = async (argv) => {
    calls.push(argv);
    if (argv[0] === "herdr-app") return spaceNew;
    if (argv[1] === "wait") return ok(waits.shift());
    if (argv[1] === "get") return ok(gets.shift() ?? { state: "live", status: null });
    if (argv[1] === "read") return { code: 0, stdout: screens.shift() ?? "", stderr: "" };
    return ok({});
  };
  return { exec, calls };
};

const rec = (n = 3): RunRecord => ({ taskId: "nightly", n, firedAt: "2026-11-30T02:00:00.000Z", terminal: null, status: "starting" });

const deps = (exec: Exec, onUpdate: (r: RunRecord) => void = () => {}): DispatchDeps => ({
  exec,
  hc: "hc",
  app: "herdr-app",
  onUpdate,
  now: () => new Date("2026-11-30T02:00:00.000Z"),
  pause: async () => {},
});

const statuses = () => {
  const seen: string[] = [];
  return { seen, onUpdate: (r: RunRecord) => seen.push(r.status) };
};

describe("runTask", () => {
  test("opens an app space running the agent with the prompt as its task, waits, records done", async () => {
    const { exec, calls } = fakeExec({ waits: [{ ended: true, reason: "status", status: "done" }] });
    const { seen, onUpdate } = statuses();
    const final = await runTask(task, rec(3), deps(exec, onUpdate));

    expect(calls).toEqual([
      [
        "herdr-app", "do", "space.new",
        JSON.stringify({ name: "Nightly triage run 3", folder: "/home/me/proj", argv: ["claude", "--"], task: "triage the inbox" }),
      ],
      ["hc", "wait", "--json", "--timeout", "30", "t9"],
    ]);
    expect(final).toEqual({ ...rec(3), terminal: "t9", space: "sp1", status: "done" });
    expect(seen).toEqual(["working", "done"]);
  });

  test("prompt starting with a dash stays the agent's message after --", async () => {
    const { exec, calls } = fakeExec({ waits: [{ ended: true, reason: "exit" }] });
    await runTask({ ...task, prompt: "--dangerously-skip-permissions" }, rec(), deps(exec));
    const sent = JSON.parse(calls[0]![3]!);
    expect(sent.argv).toEqual(["claude", "--"]);
    expect(sent.task).toBe("--dangerously-skip-permissions");
  });

  test("CLAUDE_CONFIG_DIR from the scheduler env is passed through env", async () => {
    const { exec, calls } = fakeExec({ waits: [{ ended: true, reason: "exit" }] });
    await runTask(task, rec(), { ...deps(exec), configDir: "/home/me/.cfg" });
    expect(JSON.parse(calls[0]![3]!).argv).toEqual(["env", "CLAUDE_CONFIG_DIR=/home/me/.cfg", "claude", "--"]);
  });

  test("falls back to the task claudeConfigDir field", async () => {
    const { exec, calls } = fakeExec({ waits: [{ ended: true, reason: "exit" }] });
    await runTask({ ...task, claudeConfigDir: "/p/work" }, rec(), deps(exec));
    expect(JSON.parse(calls[0]![3]!).argv).toContain("CLAUDE_CONFIG_DIR=/p/work");
  });

  test("space.new error records failed without waiting", async () => {
    const { exec, calls } = fakeExec({ spaceNew: { code: 1, stdout: '{"error":"no window"}', stderr: "" } });
    const { seen, onUpdate } = statuses();
    const final = await runTask(task, rec(), deps(exec, onUpdate));
    expect(final.status).toBe("failed");
    expect(final.error).toContain("no window");
    expect(seen).toEqual(["failed"]);
    expect(calls).toHaveLength(1);
  });

  test("garbage from space.new records failed", async () => {
    const { exec } = fakeExec({ spaceNew: { code: 0, stdout: "not json", stderr: "" } });
    const { seen, onUpdate } = statuses();
    const final = await runTask(task, rec(), deps(exec, onUpdate));
    expect(final).toMatchObject({ status: "failed", error: expect.stringContaining("not json") });
    expect(seen).toEqual(["failed"]);
  });
});

describe("watchRun", () => {
  const working = (): RunRecord => ({ ...rec(), terminal: "t9", status: "working" });

  test("keeps waiting through check-in timeouts", async () => {
    const { exec, calls } = fakeExec({ waits: [{ ended: false, reason: "timeout" }, { ended: true, reason: "blocked" }] });
    expect((await watchRun(working(), deps(exec))).status).toBe("blocked");
    expect(calls.filter((c) => c[1] === "wait")).toHaveLength(2);
  });

  test("a message to the caller is a check-in, not the end", async () => {
    const { exec } = fakeExec({ waits: [{ ended: false, reason: "message", status: "working" }, { ended: true, reason: "status", status: "done" }] });
    expect((await watchRun(working(), deps(exec))).status).toBe("done");
  });

  test("startup idle before the first turn is a check-in", async () => {
    const { exec } = fakeExec({ waits: [{ ended: true, reason: "status", status: "idle", turn: 0 }, { ended: true, reason: "status", status: "done", turn: 1 }] });
    const pauses: number[] = [];
    const final = await watchRun(working(), { ...deps(exec), pause: async (ms) => { pauses.push(ms); } });
    expect(final.status).toBe("done");
    expect(pauses).toEqual([2000]);
  });

  test("startup idle still honours timeoutAt", async () => {
    const { exec } = fakeExec({ waits: [{ ended: true, reason: "status", status: "idle", turn: 0 }] });
    const final = await watchRun({ ...working(), timeoutAt: "2026-11-30T01:00:00.000Z" }, deps(exec));
    expect(final.status).toBe("timeout");
  });

  test("idle after a turn ends the run", async () => {
    const { exec } = fakeExec({ waits: [{ ended: true, reason: "status", status: "idle", turn: 1 }] });
    expect((await watchRun(working(), deps(exec))).status).toBe("idle");
  });

  test("garbage from hc wait records failed", async () => {
    const exec: Exec = async () => ({ code: 0, stdout: "???", stderr: "" });
    const { seen, onUpdate } = statuses();
    const final = await watchRun(working(), deps(exec, onUpdate));
    expect(final.status).toBe("failed");
    expect(seen).toEqual(["failed"]);
  });

  test("hc status blocked during a check-in records blocked once, then the end status", async () => {
    const { exec } = fakeExec({
      waits: [{ ended: false, reason: "timeout" }, { ended: false, reason: "timeout" }, { ended: true, reason: "status", status: "done" }],
      gets: [blockedGet, blockedGet],
    });
    const { seen, onUpdate } = statuses();
    const final = await watchRun(working(), deps(exec, onUpdate));
    expect(seen).toEqual(["blocked", "done"]);
    expect(final.status).toBe("done");
  });

  const trust = "Is this a project you trust?\n ❯ No, exit\n   Yes, I trust this folder\n\n Enter to confirm · Esc to cancel\n\n\n";

  test("a confirm prompt in the last lines counts as blocked when the agent is not working", async () => {
    const { exec } = fakeExec({ waits: [{ ended: false, reason: "timeout" }, { ended: true, reason: "exit" }], screens: [trust] });
    const { seen, onUpdate } = statuses();
    await watchRun(working(), deps(exec, onUpdate));
    expect(seen).toEqual(["blocked", "exit"]);
  });

  test("a confirm prompt is ignored while the agent reports working", async () => {
    const { exec } = fakeExec({
      waits: [{ ended: false, reason: "timeout" }, { ended: true, reason: "exit" }],
      gets: [{ state: "live", status: { app: "claude-code", state: "working" } }],
      screens: [trust],
    });
    const { seen, onUpdate } = statuses();
    await watchRun(working(), deps(exec, onUpdate));
    expect(seen).toEqual(["exit"]);
  });

  test("a confirm prompt scrolled far above the bottom is ignored", async () => {
    const { exec } = fakeExec({
      waits: [{ ended: false, reason: "timeout" }, { ended: true, reason: "exit" }],
      screens: [trust + "line\n".repeat(10)],
    });
    const { seen, onUpdate } = statuses();
    await watchRun(working(), deps(exec, onUpdate));
    expect(seen).toEqual(["exit"]);
  });

  test("past timeoutAt the run is killed and recorded timeout", async () => {
    const { exec, calls } = fakeExec({ waits: [{ ended: false, reason: "timeout" }, { ended: false, reason: "timeout" }] });
    const times = ["2026-11-30T02:00:30.000Z", "2026-11-30T02:01:01.000Z"].map((t) => new Date(t));
    const { seen, onUpdate } = statuses();
    const final = await watchRun({ ...working(), timeoutAt: "2026-11-30T02:01:00.000Z" }, { ...deps(exec, onUpdate), now: () => times.shift()! });
    expect(final.status).toBe("timeout");
    expect(seen).toEqual(["timeout"]);
    expect(calls).toContainEqual(["hc", "kill", "t9"]);
  });
});

describe("runTask timeout", () => {
  test("timeoutMinutes sets timeoutAt from now", async () => {
    const { exec } = fakeExec({ waits: [{ ended: true, reason: "exit" }] });
    const final = await runTask({ ...task, timeoutMinutes: 45 }, rec(), deps(exec));
    expect(final.timeoutAt).toBe("2026-11-30T02:45:00.000Z");
  });
});
