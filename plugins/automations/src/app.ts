import { ACTIVE, parseJson, runTask, watchRun, type DispatchDeps, type Exec, type RunRecord } from "./dispatch";
import type { Task } from "./schema";
import { disableTasks, loadRuns, loadTasks, reserveRun, saveRun } from "./store";
import { planTick, type TickState } from "./tick";

export type AppDeps = {
  files: { tasks: string; runs: string };
  exec: Exec;
  hc: string;
  app: string;
  now: () => Date;
  log: (msg: string) => void;
  configDir?: string;
};

const trackingDeps = (deps: AppDeps): DispatchDeps => ({
  ...deps,
  onUpdate: (r) => {
    saveRun(deps.files.runs, r);
    deps.log(`${r.taskId} #${r.n} ${r.status}${r.terminal ? ` ${r.terminal}` : ""}${r.error ? `: ${r.error}` : ""}`);
  },
});

const dispatch = async (task: Task, deps: AppDeps): Promise<RunRecord | undefined> => {
  try {
    const last = loadRuns(deps.files.runs).filter((r) => r.taskId === task.id).at(-1);
    if (last && ACTIVE.includes(last.status)) {
      deps.log(`skip ${task.id}: run #${last.n} still ${last.status}`);
      if (task.schedule.type === "once") disableTasks(deps.files.tasks, [task.id]);
      return undefined;
    }
    const record = reserveRun(deps.files.runs, { taskId: task.id, firedAt: deps.now().toISOString(), status: "starting" });
    if (task.schedule.type === "once") disableTasks(deps.files.tasks, [task.id]);
    deps.log(`fire ${task.id} #${record.n}`);
    return await runTask(task, record, trackingDeps(deps));
  } catch (e) {
    deps.log(`${task.id} crashed: ${e}`);
    return undefined;
  }
};

export type StepResult = { state: TickState; runs: Promise<RunRecord | undefined>[] };

export const step = async (prev: TickState, deps: AppDeps): Promise<StepResult> => {
  const loaded = loadTasks(deps.files.tasks);
  if (!loaded.success) {
    deps.log(`tasks.json invalid, keeping previous schedule: ${loaded.error}`);
    return { state: prev, runs: [] };
  }
  const plan = planTick(loaded.data.tasks, prev, deps.now());
  for (const { task, at } of plan.missed) {
    deps.log(`missed ${task.id} ${at}`);
    if (task.schedule.type !== "once") continue;
    if (!loadRuns(deps.files.runs).some((r) => r.taskId === task.id))
      reserveRun(deps.files.runs, { taskId: task.id, firedAt: at, status: "missed" });
    disableTasks(deps.files.tasks, [task.id]);
  }
  return { state: plan.state, runs: plan.fire.map((task) => dispatch(task, deps)) };
};

export const reconcile = async (deps: AppDeps): Promise<Promise<RunRecord>[]> => {
  const tracking = trackingDeps(deps);
  const watches: Promise<RunRecord>[] = [];
  for (const run of loadRuns(deps.files.runs).filter((r) => ACTIVE.includes(r.status))) {
    const got = run.terminal ? await deps.exec([deps.hc, "get", "--json", run.terminal]) : undefined;
    const live = got?.code === 0 && parseJson<{ state?: string }>(got.stdout)?.state === "live";
    if (live) watches.push(watchRun(run, tracking));
    else tracking.onUpdate({ ...run, status: "exit" });
  }
  return watches;
};

export const runNow = async (id: string, deps: AppDeps): Promise<RunRecord | undefined> => {
  const loaded = loadTasks(deps.files.tasks);
  if (!loaded.success) throw new Error(loaded.error);
  const task = loaded.data.tasks.find((t) => t.id === id);
  if (!task) throw new Error(`unknown task: ${id}`);
  return dispatch(task, deps);
};
