package schedule

import (
	"testing"

	"github.com/tiagokriok/herdr-triggers/internal/task"
)

func mk(over func(*task.Task)) task.Task {
	zero := 0
	t := task.Task{ID: "t1", Title: "T1", Prompt: "do it", Cwd: "/tmp", Schedule: cronS("0 9 * * *"), Enabled: true, Agent: "claude", CatchUpMinutes: &zero}
	if over != nil {
		over(&t)
	}
	return t
}

func catchUp(n int) func(*task.Task) { return func(t *task.Task) { t.CatchUpMinutes = &n } }

func once(runAt string, n int) func(*task.Task) {
	return func(t *task.Task) { t.Schedule = task.Schedule{Type: "once", RunAt: runAt}; t.CatchUpMinutes = &n }
}

func ids(ts []task.Task) []string {
	out := []string{}
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

func sameIDs(t *testing.T, got []task.Task, want ...string) {
	t.Helper()
	g := ids(got)
	if len(g) != len(want) {
		t.Fatalf("want %v, got %v", want, g)
	}
	for i := range g {
		if g[i] != want[i] {
			t.Fatalf("want %v, got %v", want, g)
		}
	}
}

func armedAt(t *testing.T, s State, want string) {
	t.Helper()
	got := s["t1"].At
	if want == "" {
		if got != nil {
			t.Fatalf("want unarmed, got %v", got)
		}
		return
	}
	if got == nil || !got.Equal(at(want)) {
		t.Fatalf("want %s, got %v", want, got)
	}
}

func TestPlan(t *testing.T) {
	t.Run("first tick only arms", func(t *testing.T) {
		p := Plan([]task.Task{mk(nil)}, State{}, at("2026-11-14T08:00:00Z"))
		sameIDs(t, p.Fire)
		armedAt(t, p.State, "2026-11-14T09:00:00Z")
	})
	t.Run("fires when due and re-arms", func(t *testing.T) {
		s := Plan([]task.Task{mk(nil)}, State{}, at("2026-11-14T08:00:00Z")).State
		p := Plan([]task.Task{mk(nil)}, s, at("2026-11-14T09:00:05Z"))
		sameIDs(t, p.Fire, "t1")
		armedAt(t, p.State, "2026-11-15T09:00:00Z")
	})
	t.Run("missed after downtime, advances", func(t *testing.T) {
		s := Plan([]task.Task{mk(nil)}, State{}, at("2026-11-14T08:00:00Z")).State
		p := Plan([]task.Task{mk(nil)}, s, at("2026-11-16T12:00:00Z"))
		sameIDs(t, p.Fire)
		if len(p.Missed) != 1 || !p.Missed[0].At.Equal(at("2026-11-14T09:00:00Z")) {
			t.Fatalf("%+v", p.Missed)
		}
		armedAt(t, p.State, "2026-11-17T09:00:00Z")
	})
	t.Run("monthly 1st fires on Dec 1st", func(t *testing.T) {
		m := mk(func(t *task.Task) { t.Schedule = cronS("0 0 1 * *") })
		s := Plan([]task.Task{m}, State{}, at("2026-11-30T23:59:00Z")).State
		sameIDs(t, Plan([]task.Task{m}, s, at("2026-12-01T00:00:01Z")).Fire, "t1")
	})
	t.Run("once fires once", func(t *testing.T) {
		o := mk(once("2026-11-14T10:00:00Z", 0))
		s := Plan([]task.Task{o}, State{}, at("2026-11-14T09:59:00Z")).State
		p := Plan([]task.Task{o}, s, at("2026-11-14T10:00:02Z"))
		sameIDs(t, p.Fire, "t1")
		sameIDs(t, Plan([]task.Task{o}, p.State, at("2026-11-14T10:00:04Z")).Fire)
	})
	t.Run("disabled never fires and drops from state", func(t *testing.T) {
		p := Plan([]task.Task{mk(func(t *task.Task) { t.Enabled = false })}, State{"t1": {}}, at("2026-11-14T09:00:00Z"))
		if _, ok := p.State["t1"]; ok || len(p.Fire) > 0 {
			t.Fatal("disabled task kept")
		}
	})
	t.Run("schedule edit re-arms", func(t *testing.T) {
		s := Plan([]task.Task{mk(nil)}, State{}, at("2026-11-14T08:00:00Z")).State
		edited := mk(func(t *task.Task) { t.Schedule = cronS("0 12 * * *") })
		p := Plan([]task.Task{edited}, s, at("2026-11-14T09:00:01Z"))
		sameIDs(t, p.Fire)
		armedAt(t, p.State, "2026-11-14T12:00:00Z")
	})
	t.Run("once already past when first seen is missed", func(t *testing.T) {
		p := Plan([]task.Task{mk(once("2026-11-14T10:00:00Z", 0))}, State{}, at("2026-11-14T10:02:00Z"))
		sameIDs(t, p.Fire)
		if len(p.Missed) != 1 {
			t.Fatal(p.Missed)
		}
	})
	t.Run("cron first seen reports nothing missed", func(t *testing.T) {
		if len(Plan([]task.Task{mk(nil)}, State{}, at("2026-11-14T10:00:00Z")).Missed) != 0 {
			t.Fatal("missed")
		}
	})
	t.Run("catch-up within window fires on wake", func(t *testing.T) {
		c := mk(catchUp(120))
		s := Plan([]task.Task{c}, State{}, at("2026-11-14T08:00:00Z")).State
		p := Plan([]task.Task{c}, s, at("2026-11-14T10:59:00Z"))
		sameIDs(t, p.Fire, "t1")
		armedAt(t, p.State, "2026-11-15T09:00:00Z")
	})
	t.Run("catch-up past window is missed", func(t *testing.T) {
		c := mk(catchUp(120))
		s := Plan([]task.Task{c}, State{}, at("2026-11-14T08:00:00Z")).State
		p := Plan([]task.Task{c}, s, at("2026-11-14T11:01:00Z"))
		sameIDs(t, p.Fire)
		if len(p.Missed) != 1 {
			t.Fatal(p.Missed)
		}
	})
	t.Run("once first seen within window fires", func(t *testing.T) {
		sameIDs(t, Plan([]task.Task{mk(once("2026-11-14T10:00:00Z", 30))}, State{}, at("2026-11-14T10:20:00Z")).Fire, "t1")
	})
	t.Run("webhook never fires on the clock", func(t *testing.T) {
		h := mk(func(t *task.Task) { t.Schedule = task.Schedule{Type: "webhook", Token: "0123456789abcdef"} })
		s := Plan([]task.Task{h}, State{}, at("2026-11-14T08:00:00Z")).State
		p := Plan([]task.Task{h}, s, at("2026-11-15T08:00:00Z"))
		if len(p.Fire)+len(p.Missed) != 0 {
			t.Fatal("webhook fired")
		}
	})
}
