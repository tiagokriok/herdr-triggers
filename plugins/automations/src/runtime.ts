import { dirname } from "node:path";
import type { AppDeps } from "./app";
import type { DaemonDeps } from "./daemon";
import type { Exec } from "./dispatch";
import { paths } from "./paths";

export const exec: Exec = async (argv) => {
  const proc = Bun.spawn(argv, { stdout: "pipe", stderr: "pipe" });
  const [stdout, stderr, code] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
    proc.exited,
  ]);
  return { code, stdout, stderr };
};

const binaries = () => ({
  hc: process.env.AUTOMATIONS_HC ?? "herdr-core",
  app: process.env.AUTOMATIONS_APP ?? "herdr-app",
  configDir: process.env.CLAUDE_CONFIG_DIR,
});

export const runtimeDeps = (): AppDeps => ({
  ...binaries(),
  files: paths(),
  exec,
  now: () => new Date(),
  log: (msg) => console.log(`${new Date().toISOString()} ${msg}`),
});

export const daemonDeps = (): DaemonDeps => ({
  ...binaries(),
  exec,
  file: paths().scheduler,
  cliFile: paths().cli,
  bun: process.execPath,
  pluginDir: dirname(import.meta.dir),
});
