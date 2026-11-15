import { describe, expect, test } from "bun:test";
import { agentArgv } from "../src/agents";
import { TaskSchema } from "../src/schema";

const task = (extra: Record<string, unknown> = {}) =>
  TaskSchema.parse({ id: "t", title: "T", prompt: "p", cwd: "/w", schedule: { type: "cron", expr: "0 9 * * *" }, enabled: true, ...extra });

describe("agentArgv", () => {
  test("claude defaults to a bare -- before the prompt", () => {
    expect(agentArgv(task())).toEqual(["claude", "--"]);
  });

  test("claude gets --model and CLAUDE_CONFIG_DIR", () => {
    expect(agentArgv(task({ model: "sonnet" }), "/cfg")).toEqual(["env", "CLAUDE_CONFIG_DIR=/cfg", "claude", "--model", "sonnet", "--"]);
  });

  test("claudeConfigDir field is the fallback, ignored for other agents", () => {
    expect(agentArgv(task({ claudeConfigDir: "/p" }))).toContain("CLAUDE_CONFIG_DIR=/p");
    expect(agentArgv(task({ agent: "codex", claudeConfigDir: "/p" }), "/cfg")).toEqual(["codex", "--"]);
  });

  test("codex takes -m", () => {
    expect(agentArgv(task({ agent: "codex", model: "o3" }))).toEqual(["codex", "-m", "o3", "--"]);
  });

  test("pi takes --model", () => {
    expect(agentArgv(task({ agent: "pi", model: "anthropic/x" }))).toEqual(["pi", "--model", "anthropic/x", "--"]);
  });

  test("opencode receives the prompt through --prompt", () => {
    expect(agentArgv(task({ agent: "opencode", model: "a/b" }))).toEqual(["opencode", "--model", "a/b", "--prompt"]);
  });

  test("unknown agent: model as --model, prompt last", () => {
    expect(agentArgv(task({ agent: "gemini", model: "g" }))).toEqual(["gemini", "--model", "g"]);
  });

  test("agentArgs go before the prompt separator", () => {
    expect(agentArgv(task({ agentArgs: ["--verbose"] }))).toEqual(["claude", "--verbose", "--"]);
  });
});
