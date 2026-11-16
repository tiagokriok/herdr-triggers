import { View, div, svg } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { exec, home, readTextFile, run, spaces, theme } from "herdr";

const POLL_MS = 10000;
const ICON = "mods/triggers/icon.svg";
const CLI_FILE = "/.config/herdr-triggers/cli.json";
const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

const pad = (n) => String(n).padStart(2, "0");

function when(iso) {
  if (!iso) return "";
  const d = new Date(iso);
  const now = new Date();
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  if (d.toDateString() === now.toDateString()) return `today ${time}`;
  if (Math.abs(d - now) < 6 * 86400000) return `${DAYS[d.getDay()]} ${time}`;
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${time}`;
}

function statusColor(t, status) {
  return {
    working: t.working,
    blocked: t.blocked,
    done: t.done,
    starting: t.working,
    failed: t.ansi1,
    error: t.ansi1,
    timeout: t.ansi1,
    missed: t.unknown,
  }[status] ?? t.idle;
}

export default class Triggers extends View {
  init(_props, cx) {
    this.tasks = [];
    this.scheduler = null;
    this.problem = null;
    this.update = null;
    this.busy = {};
    cx.timer.every(POLL_MS, (cx) => this.refresh(cx));
    this.refresh(cx);
  }

  async cli(args) {
    const argv = this.argv;
    const r = await exec(argv[0], [...argv.slice(1), ...args], { timeout: 20000 });
    if (r.code !== 0) throw new Error(r.stderr.trim() || r.stdout.trim() || `exit ${r.code}`);
    return r.stdout;
  }

  refresh(cx) {
    cx.spawn(async (cx) => {
      try {
        const cfg = await readTextFile(home() + CLI_FILE);
        if (!cfg) {
          this.problem = "setup";
          return cx.notify();
        }
        this.argv = JSON.parse(cfg).argv;
        const [list, status] = await Promise.all([this.cli(["list", "--json"]), this.cli(["status", "--json"])]);
        const view = JSON.parse(list);
        this.tasks = view.tasks;
        this.update = view.update ?? null;
        this.scheduler = JSON.parse(status);
        this.problem = view.error ? `tasks.json: ${view.error}` : null;
      } catch (e) {
        this.problem = `Can't run herdr-triggers: ${e?.message ?? e}`;
      }
      cx.notify();
    });
  }

  act(key, args, cx) {
    if (this.busy[key]) return;
    this.busy[key] = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        await this.cli(args);
      } catch (e) {
        this.problem = String(e?.message ?? e);
      }
      delete this.busy[key];
      this.refresh(cx);
    });
  }

  button(id, label, onClick, enabled = true) {
    const t = theme();
    return div().id(id).px(7).py(2).rounded(5).text_size(11.5).border(1).border_color(t.border)
      .text_color(enabled ? t.foreground : t.dim)
      .when(enabled, (el) => el.hover((s) => s.bg(t.hover)).on_click((_e, cx) => onClick(cx)))
      .child(label);
  }

  header() {
    const t = theme();
    const live = this.scheduler?.state === "live";
    return h_flex().px(12).h(38).gap(8).items_center().flex_none().border_b(1).border_color(t.border)
      .child(svg(ICON).size(14).text_color(t.muted))
      .child(div().flex_1().font_semibold().child("Triggers"))
      .child(h_flex().gap(5).items_center().text_size(11.5).text_color(t.muted)
        .child(div().size(7).rounded(4).bg(live ? t.done : t.dim))
        .child(this.scheduler ? (live ? "scheduler live" : "scheduler stopped") : "…"))
      .when(Boolean(this.scheduler) && !live, (el) =>
        el.child(this.button("start", this.busy.start ? "Starting…" : "Start", (cx) => this.act("start", ["start"], cx), !this.busy.start)));
  }

  row(task) {
    const t = theme();
    const open = task.last?.space && spaces().some((s) => s.id === task.last.space);
    const last = task.last;
    return v_flex().id(`task-${task.id}`).px(12).py(9).gap(5).border_b(1).border_color(t.border)
      .child(h_flex().gap(8).items_center()
        .child(div().flex_1().truncate().font_semibold().text_color(task.enabled ? t.foreground : t.muted).child(task.title))
        .when(Boolean(last), (el) => el.child(h_flex().gap(5).items_center().text_size(11.5).text_color(statusColor(t, last.status))
          .child(div().size(7).rounded(4).bg(statusColor(t, last.status)))
          .child(last.status))))
      .child(div().text_size(11.5).text_color(t.muted).truncate().child(
        [task.schedule, task.model ? `${task.agent} · ${task.model}` : task.agent].join(" · ")))
      .child(div().text_size(11.5).text_color(t.dim).truncate().child(
        [task.enabled ? (task.next ? `next ${when(task.next)}` : "no next run") : "paused", last ? `last #${last.n} ${when(last.firedAt)}` : "never ran"].join(" · ")))
      .child(h_flex().gap(6).pt(2)
        .child(this.button(`run-${task.id}`, this.busy[`run-${task.id}`] ? "Starting…" : "Run now",
          (cx) => this.act(`run-${task.id}`, ["run-now", task.id, "--bg"], cx), !this.busy[`run-${task.id}`]))
        .child(this.button(`toggle-${task.id}`, task.enabled ? "Disable" : "Enable",
          (cx) => this.act(`toggle-${task.id}`, [task.enabled ? "disable" : "enable", task.id], cx), !this.busy[`toggle-${task.id}`]))
        .when(Boolean(open), (el) => el.child(this.button(`focus-${task.id}`, "Show run", () => run("space.select", { id: last.space })))));
  }

  empty() {
    const t = theme();
    const text = this.problem === "setup"
      ? "herdr-triggers isn't set up. Install the herdr-triggers binary and run `herdr-triggers mod install` (or `service install`). It writes ~/.config/herdr-triggers/cli.json, which this panel reads."
      : this.problem ?? "No tasks yet. Add one with `herdr-triggers tui`, or ask your agent to use the schedule-local skill.";
    return div().p(12).text_size(12).text_color(t.muted).child(text);
  }

  render() {
    const t = theme();
    const body = this.tasks.length ? v_flex().children(this.tasks.map((task) => this.row(task))) : this.empty();
    return v_flex().id("root").size_full().bg(t.surface).text_size(12.5).text_color(t.foreground)
      .child(this.header())
      .when(Boolean(this.problem) && this.tasks.length > 0, (el) =>
        el.child(div().px(12).py(6).text_size(11.5).text_color(t.blocked).child(this.problem)))
      .when(Boolean(this.update), (el) =>
        el.child(div().px(12).py(6).border_b(1).border_color(t.border).text_size(11.5).text_color(t.muted)
          .child(`${this.update.latest} available: ${this.update.command}`)))
      .child(v_flex().flex_1().min_h(0).overflow_y_scrollbar().child(body));
  }
}
