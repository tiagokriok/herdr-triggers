import type { Task } from "./schema";

type Profile = { modelFlag: string; promptPrefix: string[]; configEnv?: string };

const PROFILES: Record<string, Profile> = {
  claude: { modelFlag: "--model", promptPrefix: ["--"], configEnv: "CLAUDE_CONFIG_DIR" },
  codex: { modelFlag: "-m", promptPrefix: ["--"] },
  pi: { modelFlag: "--model", promptPrefix: ["--"] },
  opencode: { modelFlag: "--model", promptPrefix: ["--prompt"] },
};

const GENERIC: Profile = { modelFlag: "--model", promptPrefix: [] };

export const envArgv = (configDir: string | undefined, argv: string[]) =>
  configDir ? ["env", `CLAUDE_CONFIG_DIR=${configDir}`, ...argv] : argv;

export const agentArgv = (task: Task, configDir?: string): string[] => {
  const profile = PROFILES[task.agent] ?? GENERIC;
  const argv = [
    task.agent,
    ...(task.model ? [profile.modelFlag, task.model] : []),
    ...(task.agentArgs ?? []),
    ...profile.promptPrefix,
  ];
  return profile.configEnv ? envArgv(configDir ?? task.claudeConfigDir, argv) : argv;
};
