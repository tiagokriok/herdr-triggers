package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/task"
)

func TestFormRoundTrip(t *testing.T) {
	n := 30
	orig := task.Task{ID: "x", Title: "X", Prompt: "p", Cwd: "/w", Schedule: task.Schedule{Type: "cron", Expr: "0 9 * * *"}, Enabled: true, Agent: "pi", Model: "m", TimeoutMinutes: 10, CatchUpMinutes: &n, AgentArgs: []string{"-v", "--x"}}
	f := fromTask(orig)
	got, err := f.toTask()
	if err != nil {
		t.Fatal(err)
	}
	if got.Schedule != orig.Schedule || got.Agent != "pi" || got.Model != "m" || got.TimeoutMinutes != 10 || *got.CatchUpMinutes != 30 || strings.Join(got.AgentArgs, " ") != "-v --x" {
		t.Fatalf("%+v", got)
	}
}

func TestFormOtherAgentAndWebhookToken(t *testing.T) {
	f := form{ID: "h", Title: "H", Prompt: "p", Cwd: "/w", Trigger: "webhook", Agent: otherAgent, OtherAgent: "gemini", Enabled: true, CatchUp: "120"}
	got, err := f.toTask()
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent != "gemini" || len(got.Schedule.Token) < task.MinTokenLength {
		t.Fatalf("%+v", got)
	}
}

func TestFormOnceLocalTime(t *testing.T) {
	time.Local = time.FixedZone("BRT", -3*3600)
	defer func() { time.Local = time.UTC }()
	f := form{ID: "o", Title: "O", Prompt: "p", Cwd: "/w", Trigger: "once", RunAt: "2026-11-20 09:30", Agent: "claude", Enabled: true, CatchUp: "120"}
	got, err := f.toTask()
	if err != nil || got.Schedule.RunAt != "2026-11-20T09:30:00-03:00" {
		t.Fatal(got.Schedule, err)
	}
	if back := fromTask(got); back.RunAt != "2026-11-20 09:30" {
		t.Fatal(back.RunAt)
	}
}

func TestFormErrors(t *testing.T) {
	base := form{ID: "o", Title: "O", Prompt: "p", Cwd: "/w", Trigger: "cron", Cron: "0 9 * * *", Agent: "claude", CatchUp: "120"}
	for name, mod := range map[string]func(*form){
		"cwd":     func(f *form) { f.Cwd = "rel" },
		"cron":    func(f *form) { f.Cron = "bad" },
		"timeout": func(f *form) { f.Timeout = "abc" },
		"runAt":   func(f *form) { f.Trigger, f.RunAt = "once", "tomorrow" },
	} {
		t.Run(name, func(t *testing.T) {
			f := base
			mod(&f)
			if _, err := f.toTask(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestPreview(t *testing.T) {
	time.Local = time.UTC
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	if got := preview(task.Schedule{Type: "cron", Expr: "0 9 * * *"}, now); got != "next: 11-15 09:00, 11-16 09:00, 11-17 09:00" {
		t.Fatal(got)
	}
	if got := preview(task.Schedule{Type: "cron", Expr: "nope"}, now); !strings.Contains(got, "cron") {
		t.Fatal(got)
	}
}

func TestFormSignature(t *testing.T) {
	base := form{ID: "h", Title: "H", Prompt: "p", Cwd: "/w", Trigger: "webhook", Token: "0123456789abcdef0123", Agent: "claude", Enabled: true, CatchUp: "120"}

	gh := base
	gh.Signing, gh.SigSecret = "github", "whsec-0123456789abcdef"
	got, err := gh.toTask()
	if err != nil {
		t.Fatal(err)
	}
	if s := got.Schedule.Signature; s == nil || s.Header != "X-Hub-Signature-256" || s.Prefix != "sha256=" || s.Secret != gh.SigSecret {
		t.Fatalf("github %+v", s)
	}
	if back := fromTask(got); back.Signing != "github" || back.SigSecret != gh.SigSecret {
		t.Fatalf("github round trip %+v", back)
	}

	custom := base
	custom.Signing, custom.SigSecret, custom.SigHeader, custom.SigEncoding = "custom", "whsec-0123456789abcdef", "X-Grafana-Sig", "base64"
	got, err = custom.toTask()
	if err != nil || got.Schedule.Signature.Header != "X-Grafana-Sig" || got.Schedule.Signature.Encoding != "base64" {
		t.Fatalf("custom %+v %v", got.Schedule.Signature, err)
	}
	if back := fromTask(got); back.Signing != "custom" || back.SigHeader != "X-Grafana-Sig" {
		t.Fatalf("custom round trip %+v", back)
	}

	none := base
	none.Signing, none.SigSecret = "none", "ignored-0123456789"
	if got, _ := none.toTask(); got.Schedule.Signature != nil {
		t.Fatalf("none kept a signature %+v", got.Schedule.Signature)
	}

	short := base
	short.Signing, short.SigSecret = "linear", "short"
	if _, err := short.toTask(); err == nil {
		t.Fatal("short secret accepted")
	}
}
