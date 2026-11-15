import { reconcile, step } from "./app";
import { runtimeDeps } from "./runtime";
import type { TickState } from "./tick";

const deps = runtimeDeps();
const TICK_MS = 5_000;

deps.log(`scheduler up, tasks: ${deps.files.tasks}, runs: ${deps.files.runs}`);
await reconcile(deps);
let state: TickState = {};
for (;;) {
  try {
    const next = (await step(state, deps)).state;
    for (const [id, s] of Object.entries(next))
      if (s.at !== state[id]?.at) deps.log(`${id} next: ${s.at ?? "never"}`);
    state = next;
  } catch (e) {
    deps.log(`tick failed: ${e}`);
  }
  await Bun.sleep(TICK_MS);
}
