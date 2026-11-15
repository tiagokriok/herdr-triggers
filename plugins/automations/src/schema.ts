import { Cron } from "croner";
import { isAbsolute } from "node:path";
import { z } from "zod";

const isValidCron = (expr: string) => {
  try {
    new Cron(expr, { paused: true });
    return true;
  } catch {
    return false;
  }
};

export const ScheduleSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("cron"),
    expr: z
      .string()
      .refine((e) => e.trim().split(/\s+/).length === 5, "cron must have exactly 5 fields (no seconds)")
      .refine(isValidCron, "invalid cron expression"),
  }),
  z.object({ type: z.literal("once"), runAt: z.iso.datetime({ offset: true, local: false }) }),
]);

export const TaskSchema = z.object({
  id: z.string().regex(/^[a-z0-9][a-z0-9-]*$/),
  title: z.string().min(1),
  prompt: z.string().min(1),
  cwd: z.string().refine(isAbsolute, "cwd must be an absolute path"),
  schedule: ScheduleSchema,
  enabled: z.boolean(),
  agent: z.string().min(1).default("claude"),
  claudeConfigDir: z.string().min(1).optional(),
});

export const TasksFileSchema = z
  .object({ tasks: z.array(TaskSchema) })
  .refine((f) => new Set(f.tasks.map((t) => t.id)).size === f.tasks.length, "duplicate task ids");

export type Schedule = z.infer<typeof ScheduleSchema>;
export type Task = z.infer<typeof TaskSchema>;
export type TasksFile = z.infer<typeof TasksFileSchema>;

export type ParseResult = { success: true; data: TasksFile } | { success: false; error: string };

export const parseTasksFile = (text: string): ParseResult => {
  let json: unknown;
  try {
    json = JSON.parse(text);
  } catch (e) {
    return { success: false, error: `invalid json: ${(e as Error).message}` };
  }
  const result = TasksFileSchema.safeParse(json);
  return result.success
    ? { success: true, data: result.data }
    : { success: false, error: z.prettifyError(result.error) };
};
