import { beforeEach, describe, expect, test } from "bun:test";
import { existsSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { schedulerStatus, startScheduler, stopScheduler, type DaemonDeps } from "../src/daemon";
import type { Exec } from "../src/dispatch";

let file: string;
let cliFile: string;
let calls: string[][];
let live: Record<string, string[]>;

const exec: Exec = async (argv) => {
  calls.push(argv);
  const out = (o: unknown) => ({ code: 0, stdout: JSON.stringify(o), stderr: "" });
  if (argv[0] === "herdr-app" && argv[2] === "space.new") return out({ space: "spNEW", tab: "tbNEW", terminal: "tNEW" });
  if (argv[0] === "herdr-app") return out({ ok: true });
  if (argv[1] === "get") {
    const fg = live[argv.at(-1)!];
    return fg ? out({ state: "live", argv: ["zsh"], foreground: { argv: fg } }) : { code: 1, stdout: "", stderr: "not_found" };
  }
  return out({});
};

const deps = (configDir?: string): DaemonDeps => ({
  exec, hc: "hc", app: "herdr-app", file, cliFile, bun: "/usr/bin/bun", pluginDir: "/opt/automations", configDir,
});

beforeEach(() => {
  const dir = mkdtempSync(join(tmpdir(), "daemon-"));
  file = join(dir, "state", "scheduler.json");
  cliFile = join(dir, "config", "cli.json");
  calls = [];
  live = {};
});

describe("startScheduler", () => {
  test("opens an Automations space through the app and remembers it", async () => {
    expect(await startScheduler(deps("/home/me/.cfg"))).toBe("started tNEW");
    expect(calls).toEqual([
      [
        "herdr-app", "do", "space.new",
        JSON.stringify({
          name: "Automations",
          folder: "/opt/automations",
          argv: ["env", "CLAUDE_CONFIG_DIR=/home/me/.cfg", "bun", "src/scheduler.ts"],
        }),
      ],
    ]);
    expect(JSON.parse(readFileSync(file, "utf8"))).toEqual({ space: "spNEW", terminal: "tNEW" });
  });

  test("writes cli.json so the app mod can find the CLI", async () => {
    await startScheduler(deps());
    expect(JSON.parse(readFileSync(cliFile, "utf8"))).toEqual({ argv: ["/usr/bin/bun", "/opt/automations/src/cli.ts"] });
  });

  test("is idempotent while the remembered terminal runs the scheduler", async () => {
    await startScheduler(deps());
    writeFileSync(file, JSON.stringify({ space: "sp1", terminal: "t1" }));
    calls = [];
    live.t1 = ["bun", "src/scheduler.ts"];
    expect(await startScheduler(deps())).toBe("already running t1");
    expect(calls.every((c) => c[0] === "hc" && c[1] === "get")).toBe(true);
  });

  test("replaces a dead or shell-only scheduler space", async () => {
    await startScheduler(deps());
    writeFileSync(file, JSON.stringify({ space: "sp1", terminal: "t1" }));
    calls = [];
    live.t1 = ["/usr/bin/zsh"];
    expect(await startScheduler(deps())).toBe("started tNEW");
    expect(calls).toContainEqual(["herdr-app", "do", "space.close", "--id", "sp1", "--confirmed", "true"]);
  });

  test("fails loudly when the app refuses", async () => {
    const refusing: Exec = async () => ({ code: 1, stdout: '{"error":"no window"}', stderr: "" });
    expect(startScheduler({ ...deps(), exec: refusing })).rejects.toThrow("no window");
  });
});

describe("stopScheduler", () => {
  test("closes the remembered space and forgets it", async () => {
    await startScheduler(deps());
    calls = [];
    expect(await stopScheduler(deps())).toBe("stopped");
    expect(calls).toEqual([["herdr-app", "do", "space.close", "--id", "spNEW", "--confirmed", "true"]]);
    expect(existsSync(file)).toBe(false);
  });

  test("nothing remembered is a no-op", async () => {
    expect(await stopScheduler(deps())).toBe("not running");
    expect(calls).toEqual([]);
  });
});

describe("schedulerStatus", () => {
  test("live while the remembered terminal runs the scheduler", async () => {
    await startScheduler(deps());
    live.tNEW = ["bun", "src/scheduler.ts"];
    expect(await schedulerStatus(deps())).toEqual({ state: "live", space: "spNEW", terminal: "tNEW" });
  });

  test("stopped when nothing is remembered or the terminal is gone", async () => {
    expect(await schedulerStatus(deps())).toEqual({ state: "stopped" });
    await startScheduler(deps());
    expect(await schedulerStatus(deps())).toEqual({ state: "stopped" });
  });
});
