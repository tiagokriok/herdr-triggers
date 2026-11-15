package dispatch

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
)

var base = task.Task{ID: "nightly", Title: "Nightly triage", Prompt: "triage the inbox", Cwd: "/w/proj", Schedule: task.Schedule{Type: "cron", Expr: "0 2 * * *"}, Enabled: true, Agent: "claude"}

func js(v any) Result { b, _ := json.Marshal(v); return Result{Stdout: string(b)} }

type fake struct {
	waits, gets []any
	screens     []string
	spaceNew    *Result
	calls       [][]string
}

func pop[T any](q *[]T) (T, bool) {
	var zero T
	if len(*q) == 0 {
		return zero, false
	}
	v := (*q)[0]
	*q = (*q)[1:]
	return v, true
}

func (f *fake) exec(argv []string) Result {
	f.calls = append(f.calls, argv)
	switch {
	case argv[0] == "herdr-app":
		if f.spaceNew != nil {
			return *f.spaceNew
		}
		return js(map[string]string{"space": "sp1", "tab": "tb1", "terminal": "t9"})
	case argv[1] == "wait":
		w, _ := pop(&f.waits)
		return js(w)
	case argv[1] == "get":
		if g, ok := pop(&f.gets); ok {
			return js(g)
		}
		return js(map[string]any{"state": "live", "status": nil})
	case argv[1] == "read":
		s, _ := pop(&f.screens)
		return Result{Stdout: s}
	}
	return js(map[string]any{})
}

const now = "2026-11-30T02:00:00Z"

func mustTime(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

func deps(f *fake, seen *[]string) Deps {
	return Deps{
		Exec: f.exec, HC: "hc", App: "herdr-app",
		OnUpdate: func(r store.Run) {
			if seen != nil {
				*seen = append(*seen, r.Status)
			}
		},
		Now:   func() time.Time { return mustTime(now) },
		Pause: func(time.Duration) {},
	}
}

func rec() store.Run {
	return store.Run{TaskID: "nightly", N: 3, FiredAt: mustTime(now), Status: "starting"}
}

func working() store.Run { r := rec(); r.Terminal, r.Status = "t9", "working"; return r }

type w = map[string]any

func TestRunTaskOpensSpaceAndWaits(t *testing.T) {
	f := &fake{waits: []any{w{"ended": true, "reason": "status", "status": "done"}}}
	var seen []string
	final := RunTask(base, rec(), deps(f, &seen))
	payload, _ := json.Marshal(map[string]any{"name": "Nightly triage run 3", "folder": "/w/proj", "argv": []string{"claude", "--"}, "task": "triage the inbox"})
	want := [][]string{{"herdr-app", "do", "space.new", string(payload)}, {"hc", "wait", "--json", "--timeout", "30", "t9"}}
	if len(f.calls) != 2 || !slices.Equal(f.calls[0], want[0]) || !slices.Equal(f.calls[1], want[1]) {
		t.Fatalf("calls %q", f.calls)
	}
	if final.Terminal != "t9" || final.Space != "sp1" || final.Status != "done" || !slices.Equal(seen, []string{"working", "done"}) {
		t.Fatalf("%+v %v", final, seen)
	}
}

func sent(f *fake) map[string]any {
	var m map[string]any
	json.Unmarshal([]byte(f.calls[0][3]), &m)
	return m
}

func TestRunTaskArgvAndPrompt(t *testing.T) {
	f := &fake{waits: []any{w{"ended": true, "reason": "exit"}}}
	tk := base
	tk.Prompt = "--dangerously-skip-permissions"
	d := deps(f, nil)
	d.ConfigDir = "/cfg"
	RunTask(tk, rec(), d)
	m := sent(f)
	if m["task"] != "--dangerously-skip-permissions" || m["argv"].([]any)[1] != "CLAUDE_CONFIG_DIR=/cfg" {
		t.Fatal(m)
	}
}

func TestRunTaskTimeoutAt(t *testing.T) {
	f := &fake{waits: []any{w{"ended": true, "reason": "exit"}}}
	tk := base
	tk.TimeoutMinutes = 45
	final := RunTask(tk, rec(), deps(f, nil))
	if final.TimeoutAt == nil || !final.TimeoutAt.Equal(mustTime("2026-11-30T02:45:00Z")) {
		t.Fatal(final.TimeoutAt)
	}
}

func TestRunTaskSpaceNewFailures(t *testing.T) {
	for name, res := range map[string]Result{
		"error":   {Code: 1, Stdout: `{"error":"no window"}`},
		"garbage": {Stdout: "not json"},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fake{spaceNew: &res}
			var seen []string
			final := RunTask(base, rec(), deps(f, &seen))
			if final.Status != "failed" || !strings.Contains(final.Error, map[string]string{"error": "no window", "garbage": "not json"}[name]) || len(f.calls) != 1 || !slices.Equal(seen, []string{"failed"}) {
				t.Fatalf("%+v %v", final, seen)
			}
		})
	}
}

func watch(t *testing.T, f *fake, r store.Run, mod func(*Deps)) (store.Run, []string) {
	t.Helper()
	var seen []string
	d := deps(f, &seen)
	if mod != nil {
		mod(&d)
	}
	return WatchRun(r, d), seen
}

var timeout = w{"ended": false, "reason": "timeout"}

func TestWatch(t *testing.T) {
	blockedGet := w{"state": "live", "status": w{"app": "claude-code", "state": "blocked"}}
	trust := "Is this a project you trust?\n ❯ No, exit\n   Yes, I trust this folder\n\n Enter to confirm · Esc to cancel\n\n\n"

	t.Run("keeps waiting through timeouts", func(t *testing.T) {
		f := &fake{waits: []any{timeout, w{"ended": true, "reason": "blocked"}}}
		if r, _ := watch(t, f, working(), nil); r.Status != "blocked" {
			t.Fatal(r)
		}
	})
	t.Run("garbage from wait fails", func(t *testing.T) {
		f := &fake{}
		f.waits = []any{"???"}
		r, seen := watch(t, &fake{waits: []any{nil}}, working(), func(d *Deps) { d.Exec = func([]string) Result { return Result{Stdout: "???"} } })
		if r.Status != "failed" || !slices.Equal(seen, []string{"failed"}) {
			t.Fatal(r, seen)
		}
	})
	t.Run("blocked once then end status", func(t *testing.T) {
		f := &fake{waits: []any{timeout, timeout, w{"ended": true, "reason": "status", "status": "done"}}, gets: []any{blockedGet, blockedGet}}
		r, seen := watch(t, f, working(), nil)
		if r.Status != "done" || !slices.Equal(seen, []string{"blocked", "done"}) {
			t.Fatal(seen)
		}
	})
	t.Run("confirm prompt on screen counts as blocked when not working", func(t *testing.T) {
		f := &fake{waits: []any{timeout, w{"ended": true, "reason": "exit"}}, screens: []string{trust}}
		if _, seen := watch(t, f, working(), nil); !slices.Equal(seen, []string{"blocked", "exit"}) {
			t.Fatal(seen)
		}
	})
	t.Run("confirm prompt ignored while working", func(t *testing.T) {
		f := &fake{waits: []any{timeout, w{"ended": true, "reason": "exit"}}, gets: []any{w{"state": "live", "status": w{"state": "working"}}}, screens: []string{trust}}
		if _, seen := watch(t, f, working(), nil); !slices.Equal(seen, []string{"exit"}) {
			t.Fatal(seen)
		}
	})
	t.Run("confirm prompt scrolled away ignored", func(t *testing.T) {
		f := &fake{waits: []any{timeout, w{"ended": true, "reason": "exit"}}, screens: []string{trust + strings.Repeat("line\n", 10)}}
		if _, seen := watch(t, f, working(), nil); !slices.Equal(seen, []string{"exit"}) {
			t.Fatal(seen)
		}
	})
	t.Run("message is a check-in", func(t *testing.T) {
		f := &fake{waits: []any{w{"ended": false, "reason": "message"}, w{"ended": true, "reason": "status", "status": "done"}}}
		if r, _ := watch(t, f, working(), nil); r.Status != "done" {
			t.Fatal(r)
		}
	})
	t.Run("startup idle turn 0 is a check-in with a pause", func(t *testing.T) {
		f := &fake{waits: []any{w{"ended": true, "reason": "status", "status": "idle", "turn": 0}, w{"ended": true, "reason": "status", "status": "done", "turn": 1}}}
		var pauses []time.Duration
		r, _ := watch(t, f, working(), func(d *Deps) { d.Pause = func(x time.Duration) { pauses = append(pauses, x) } })
		if r.Status != "done" || !slices.Equal(pauses, []time.Duration{2 * time.Second}) {
			t.Fatal(r, pauses)
		}
	})
	t.Run("idle after a turn ends", func(t *testing.T) {
		f := &fake{waits: []any{w{"ended": true, "reason": "status", "status": "idle", "turn": 1}}}
		if r, _ := watch(t, f, working(), nil); r.Status != "idle" {
			t.Fatal(r)
		}
	})
	t.Run("past timeoutAt kills and records timeout", func(t *testing.T) {
		f := &fake{waits: []any{timeout, timeout}}
		times := []time.Time{mustTime("2026-11-30T02:00:30Z"), mustTime("2026-11-30T02:01:01Z")}
		r := working()
		limit := mustTime("2026-11-30T02:01:00Z")
		r.TimeoutAt = &limit
		final, seen := watch(t, f, r, func(d *Deps) { d.Now = func() time.Time { x, _ := pop(&times); return x } })
		if final.Status != "timeout" || !slices.Equal(seen, []string{"timeout"}) || !slices.ContainsFunc(f.calls, func(c []string) bool { return slices.Equal(c, []string{"hc", "kill", "t9"}) }) {
			t.Fatal(final, seen, f.calls)
		}
	})
	t.Run("startup idle honours timeoutAt", func(t *testing.T) {
		f := &fake{waits: []any{w{"ended": true, "reason": "status", "status": "idle", "turn": 0}}}
		r := working()
		limit := mustTime("2026-11-30T01:00:00Z")
		r.TimeoutAt = &limit
		if final, _ := watch(t, f, r, nil); final.Status != "timeout" {
			t.Fatal(final)
		}
	})
}
