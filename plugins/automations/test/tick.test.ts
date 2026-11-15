import { describe, expect, test } from "bun:test";
import type { Task } from "../src/schema";
import { planTick, type TickState } from "../src/tick";

const task = (over: Partial<Task> = {}): Task => ({
  id: "t1",
  title: "T1",
  prompt: "do it",
  cwd: "/tmp",
  schedule: { type: "cron", expr: "0 9 * * *" },
  enabled: true,
  agent: "claude",
  ...over,
});

const at = (iso: string) => new Date(iso);

describe("planTick", () => {
  test("first tick only arms the next run", () => {
    const r = planTick([task()], {}, at("2026-11-14T08:00:00Z"));
    expect(r.fire).toEqual([]);
    expect(r.state.t1!.at).toBe("2026-11-14T09:00:00.000Z");
  });

  test("fires when due and re-arms", () => {
    const armed = planTick([task()], {}, at("2026-11-14T08:00:00Z")).state;
    const r = planTick([task()], armed, at("2026-11-14T09:00:05Z"));
    expect(r.fire.map((t) => t.id)).toEqual(["t1"]);
    expect(r.state.t1!.at).toBe("2026-11-15T09:00:00.000Z");
  });

  test("skips missed runs after downtime, reports them and advances", () => {
    const armed = planTick([task()], {}, at("2026-11-14T08:00:00Z")).state;
    const r = planTick([task()], armed, at("2026-11-16T12:00:00Z"));
    expect(r.fire).toEqual([]);
    expect(r.missed.map((m) => [m.task.id, m.at])).toEqual([["t1", "2026-11-14T09:00:00.000Z"]]);
    expect(r.state.t1!.at).toBe("2026-11-17T09:00:00.000Z");
  });

  test("end of month: monthly 1st job fires on Dec 1st", () => {
    const t = task({ schedule: { type: "cron", expr: "0 0 1 * *" } });
    const armed = planTick([t], {}, at("2026-11-30T23:59:30Z")).state;
    const r = planTick([t], armed, at("2026-12-01T00:00:10Z"));
    expect(r.fire.map((x) => x.id)).toEqual(["t1"]);
    expect(r.state.t1!.at).toBe("2027-01-01T00:00:00.000Z");
  });

  test("once task fires once and is marked for disable", () => {
    const t = task({ schedule: { type: "once", runAt: "2026-11-14T10:00:00Z" } });
    const armed = planTick([t], {}, at("2026-11-14T09:59:00Z")).state;
    const r1 = planTick([t], armed, at("2026-11-14T10:00:02Z"));
    expect(r1.fire.map((x) => x.id)).toEqual(["t1"]);
    const r2 = planTick([t], r1.state, at("2026-11-14T10:00:04Z"));
    expect(r2.fire).toEqual([]);
  });

  test("disabled tasks never fire and drop from state", () => {
    const armed = planTick([task()], {}, at("2026-11-14T08:00:00Z")).state;
    const r = planTick([task({ enabled: false })], armed, at("2026-11-14T09:00:01Z"));
    expect(r.fire).toEqual([]);
    expect(r.state).toEqual({});
  });

  test("schedule edit re-arms instead of firing stale time", () => {
    const armed: TickState = planTick([task()], {}, at("2026-11-14T08:00:00Z")).state;
    const edited = task({ schedule: { type: "cron", expr: "0 12 * * *" } });
    const r = planTick([edited], armed, at("2026-11-14T09:00:01Z"));
    expect(r.fire).toEqual([]);
    expect(r.state.t1!.at).toBe("2026-11-14T12:00:00.000Z");
  });

  test("once task already past when first seen is reported missed", () => {
    const t = task({ schedule: { type: "once", runAt: "2026-11-14T10:00:00Z" } });
    const r = planTick([t], {}, at("2026-11-14T10:00:02Z"));
    expect(r.fire).toEqual([]);
    expect(r.missed.map((m) => [m.task.id, m.at])).toEqual([["t1", "2026-11-14T10:00:00.000Z"]]);
  });

  test("cron first seen reports nothing missed", () => {
    expect(planTick([task()], {}, at("2026-11-14T10:00:00Z")).missed).toEqual([]);
  });
});
