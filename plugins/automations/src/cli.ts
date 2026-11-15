import { spawn } from "node:child_process";
import { runNow } from "./app";
import { listTasks, setEnabled } from "./commands";
import { schedulerStatus, startScheduler, stopScheduler } from "./daemon";
import { daemonDeps, runtimeDeps } from "./runtime";

const USAGE = "usage: bun src/cli.ts list [--json] | run-now <id> [--bg] | enable <id> | disable <id> | status [--json] | start | stop";

const deps = runtimeDeps();
const [cmd, ...rest] = process.argv.slice(2);
const json = rest.includes("--json");
const arg = rest.find((a) => !a.startsWith("--"));

const main = async () => {
  switch (cmd) {
    case "list": {
      const view = listTasks(deps.files, new Date());
      if (json) return console.log(JSON.stringify(view));
      if (view.error) throw new Error(view.error);
      for (const t of view.tasks) {
        const last = t.last ? `#${t.last.n} ${t.last.status} ${t.last.firedAt}` : "-";
        console.log(`${t.id}\t${t.title}\t${t.schedule}\tnext: ${t.enabled ? (t.next ?? "never") : "paused"}\tlast: ${last}`);
      }
      return;
    }
    case "run-now": {
      if (!arg) throw new Error(USAGE);
      if (rest.includes("--bg")) {
        spawn(process.execPath, [process.argv[1]!, "run-now", arg], { detached: true, stdio: "ignore" }).unref();
        return console.log(`started ${arg}`);
      }
      await runNow(arg, deps);
      return;
    }
    case "enable":
    case "disable":
      if (!arg) throw new Error(USAGE);
      setEnabled(deps.files.tasks, arg, cmd === "enable");
      return console.log(`${cmd}d ${arg}`);
    case "status": {
      const s = await schedulerStatus(daemonDeps());
      return console.log(json ? JSON.stringify(s) : s.state === "live" ? `live ${s.terminal}` : "stopped");
    }
    case "start":
      return console.log(await startScheduler(daemonDeps()));
    case "stop":
      return console.log(await stopScheduler(daemonDeps()));
    default:
      console.error(USAGE);
      process.exit(2);
  }
};

try {
  await main();
} catch (e) {
  console.error((e as Error).message);
  process.exit(1);
}
