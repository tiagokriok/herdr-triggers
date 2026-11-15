import { describe, expect, test } from "bun:test";
import { nextRun } from "../src/schedule";

const at = (iso: string) => new Date(iso);

describe("nextRun", () => {
  test("cron: next weekday 09:00", () => {
    expect(nextRun({ type: "cron", expr: "0 9 * * 1-5" }, at("2026-11-14T10:00:00Z"))).toEqual(
      at("2026-11-16T09:00:00Z"),
    );
  });

  test("cron: strictly after the reference time", () => {
    expect(nextRun({ type: "cron", expr: "0 9 * * *" }, at("2026-11-14T09:00:00Z"))).toEqual(
      at("2026-11-15T09:00:00Z"),
    );
  });

  test("cron: end of month rolls into the 1st", () => {
    expect(nextRun({ type: "cron", expr: "0 0 1 * *" }, at("2026-11-30T23:59:00Z"))).toEqual(
      at("2026-12-01T00:00:00Z"),
    );
  });

  test("cron: last day of month from the 1st", () => {
    expect(nextRun({ type: "cron", expr: "0 18 L * *" }, at("2026-02-01T00:00:00Z"))).toEqual(
      at("2026-02-28T18:00:00Z"),
    );
  });

  test("once: future runAt honours offset", () => {
    expect(
      nextRun({ type: "once", runAt: "2026-11-14T10:00:00-03:00" }, at("2026-11-14T12:00:00Z")),
    ).toEqual(at("2026-11-14T13:00:00Z"));
  });

  test("once: past runAt has no next run", () => {
    expect(nextRun({ type: "once", runAt: "2026-11-14T10:00:00Z" }, at("2026-11-14T10:00:00Z"))).toBeNull();
  });
});

describe("nextRun across DST (America/New_York)", () => {
  test("daily 09:00 local shifts from 14:00Z to 13:00Z on spring forward", () => {
    const tz = process.env.TZ;
    process.env.TZ = "America/New_York";
    try {
      const sched = { type: "cron", expr: "0 9 * * *" } as const;
      expect(nextRun(sched, new Date("2026-03-07T15:00:00Z"))).toEqual(new Date("2026-03-08T13:00:00Z"));
      expect(nextRun(sched, new Date("2026-03-06T15:00:00Z"))).toEqual(new Date("2026-03-07T14:00:00Z"));
    } finally {
      process.env.TZ = tz;
    }
  });
});
