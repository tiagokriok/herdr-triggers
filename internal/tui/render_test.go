package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/tiagokriok/herdr-triggers/internal/daemon"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
	"github.com/tiagokriok/herdr-triggers/internal/tunnel"
	"github.com/tiagokriok/herdr-triggers/internal/view"
)

var now = time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

func TestRelTime(t *testing.T) {
	time.Local = time.UTC
	cases := map[time.Duration]string{
		30 * time.Second:     "in <1m",
		5 * time.Minute:      "in 5m",
		3 * time.Hour:        "in 3h",
		23 * time.Hour:       "tomorrow 09:00",
		6 * 24 * time.Hour:   "Fri 11-20 10:00",
		-90 * time.Second:    "1m ago",
		-5 * time.Hour:       "5h ago",
		-20 * time.Hour:      "yesterday 14:00",
		-10 * 24 * time.Hour: "11-04 10:00",
	}
	for d, want := range cases {
		if got := relTime(now, now.Add(d)); got != want {
			t.Errorf("%s: got %q want %q", d, got, want)
		}
	}
}

func TestMask(t *testing.T) {
	if got := mask("https://b.ts.net/hooks/a/0123456789abcdef"); got != "https://b.ts.net/hooks/a/0123••••••" {
		t.Fatal(got)
	}
}

func TestQuickTunnel(t *testing.T) {
	for c, want := range map[tunnel.Config]bool{
		{Provider: "cloudflared"}:                  true,
		{Provider: "ngrok"}:                        true,
		{Provider: "ngrok", Domain: "d.ngrok.app"}: false,
		{Provider: "tailscale"}:                    false,
		{Provider: "none"}:                         false,
	} {
		if quickTunnel(c) != want {
			t.Error(c)
		}
	}
}

func screen() screenData {
	time.Local = time.UTC
	next := now.Add(23 * time.Hour)
	return screenData{
		now: now, width: 100, height: 30, cursor: 1,
		sched:  daemon.State{State: "live", Terminal: "t9"},
		public: "https://box.tail1.ts.net",
		config: tunnel.Config{Provider: "tailscale"},
		listing: view.Listing{Tasks: []view.Task{
			{ID: "daily-review", Schedule: "weekdays at 09:00", Enabled: true, Agent: "claude", Model: "opus", Next: &next, Last: &view.Last{N: 12, Status: "done", FiredAt: now.Add(-time.Hour)}},
			{ID: "grafana-alert", Schedule: "on webhook", Enabled: true, Agent: "codex", Last: &view.Last{N: 3, Status: "working", FiredAt: now.Add(-2 * time.Minute)}},
			{ID: "deploy-check", Schedule: "once on 2026-11-16 10:00", Enabled: false, Agent: "pi"},
		}},
		selected: task.Task{ID: "grafana-alert", Title: "Grafana", Prompt: "Find the cause", Cwd: "/w/app", Agent: "codex", Schedule: task.Schedule{Type: "webhook", Token: "0123456789abcdef"}},
		runs:     []store.Run{{N: 3, Status: "working", FiredAt: now.Add(-2 * time.Minute)}, {N: 2, Status: "failed", FiredAt: now.Add(-25 * time.Hour)}},
		hooks:    []store.Delivery{{Outcome: "accepted", Bytes: 15, RunN: 3, ReceivedAt: now.Add(-2 * time.Minute)}},
	}
}

func TestRender(t *testing.T) {
	out := render(screen())
	for _, want := range []string{
		"scheduler on", "https://box.tail1.ts.net",
		"ID", "TRIGGER", "STATUS",
		"daily-review", "claude·opus", "tomorrow 09:00", "#12 done",
		"#3 working", "○ off",
		"https://box.tail1.ts.net/hooks/grafana-alert/0123••••••",
		"Find the cause", "#2 failed", "accepted",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "0123456789abcdef") {
		t.Error("token not masked")
	}
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Errorf("line %d is %d wide: %q", i, w, line)
		}
	}
}

func TestRenderNarrowAndStates(t *testing.T) {
	s := screen()
	s.width = 60
	s.reveal = true
	s.sched = daemon.State{State: "stopped"}
	s.publicErr = "tailscale exited: Funnel not available"
	out := render(s)
	for _, want := range []string{"scheduler off", "456789abcdef", "Funnel not available"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line %d is %d wide: %q", i, w, line)
		}
	}
	s.listing = view.Listing{Tasks: []view.Task{}}
	if out := render(s); !strings.Contains(out, "press a to add") {
		t.Error(out)
	}
}

func TestRenderScrolls(t *testing.T) {
	s := screen()
	s.height = 16
	for i := range 30 {
		s.listing.Tasks = append(s.listing.Tasks, view.Task{ID: "t" + strings.Repeat("x", i%3) + string(rune('a'+i%26)), Schedule: "daily at 09:00", Enabled: true, Agent: "pi"})
	}
	s.cursor = 25
	out := render(s)
	if lines := strings.Count(out, "\n"); lines > 16 {
		t.Fatalf("%d lines\n%s", lines, out)
	}
	if !strings.Contains(out, "▸") {
		t.Fatal("cursor row hidden")
	}
}

func TestRenderWideKeepsColumnsClose(t *testing.T) {
	s := screen()
	s.width = 200
	head := strings.Split(render(s), "\n")[2]
	if i := strings.Index(head, "TRIGGER"); i > 40 {
		t.Fatalf("TRIGGER at %d: %q", i, head)
	}
}
