import { describe, expect, test } from "bun:test";
import { parseTasksFile } from "../src/schema";

const valid = {
  tasks: [
    {
      id: "daily-review",
      title: "Daily review",
      prompt: "review open PRs",
      cwd: "/tmp",
      schedule: { type: "cron", expr: "0 9 * * 1-5" },
      enabled: true,
    },
  ],
};

describe("parseTasksFile", () => {
  test("accepts a valid file and defaults agent to claude", () => {
    const result = parseTasksFile(JSON.stringify(valid));
    expect(result.success).toBe(true);
    if (result.success) expect(result.data.tasks[0]!.agent).toBe("claude");
  });

  test("rejects invalid json", () => {
    expect(parseTasksFile("{nope").success).toBe(false);
  });

  test("rejects a once schedule without offset", () => {
    const bad = structuredClone(valid);
    bad.tasks[0]!.schedule = { type: "once", runAt: "2026-11-14T10:00:00" } as never;
    expect(parseTasksFile(JSON.stringify(bad)).success).toBe(false);
  });

  test("rejects an invalid cron expression", () => {
    const bad = structuredClone(valid);
    bad.tasks[0]!.schedule.expr = "99 * * * *";
    const result = parseTasksFile(JSON.stringify(bad));
    expect(result.success).toBe(false);
    if (!result.success) expect(result.error).toContain("cron");
  });

  test("rejects duplicate ids", () => {
    const bad = { tasks: [valid.tasks[0], valid.tasks[0]] };
    expect(parseTasksFile(JSON.stringify(bad)).success).toBe(false);
  });

  test("rejects a relative cwd", () => {
    const bad = structuredClone(valid);
    bad.tasks[0]!.cwd = "proj";
    expect(parseTasksFile(JSON.stringify(bad)).success).toBe(false);
  });

  test("rejects cron with a seconds field", () => {
    const bad = structuredClone(valid);
    bad.tasks[0]!.schedule.expr = "0 0 9 * * 1-5";
    const result = parseTasksFile(JSON.stringify(bad));
    expect(result.success).toBe(false);
    if (!result.success) expect(result.error).toContain("5 fields");
  });
});
