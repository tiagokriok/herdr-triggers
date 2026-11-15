package tui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/schedule"
	"github.com/tiagokriok/herdr-triggers/internal/task"
)

const (
	otherAgent  = "other…"
	localLayout = "2006-01-02 15:04"
)

type form struct {
	ID, Title, Prompt, Cwd string
	Trigger                string
	Cron, RunAt, Token     string
	Agent, OtherAgent      string
	Model, AgentArgs       string
	Timeout, CatchUp       string
	Enabled                bool
}

func newToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func knownAgent(a string) bool {
	for _, k := range []string{"claude", "codex", "pi", "opencode"} {
		if a == k {
			return true
		}
	}
	return false
}

func fromTask(t task.Task) form {
	f := form{
		ID: t.ID, Title: t.Title, Prompt: t.Prompt, Cwd: t.Cwd, Trigger: t.Schedule.Type,
		Cron: t.Schedule.Expr, Token: t.Schedule.Token, Agent: t.Agent, Model: t.Model,
		AgentArgs: strings.Join(t.AgentArgs, " "), Enabled: t.Enabled, CatchUp: strconv.Itoa(task.DefaultCatchUpMinutes),
	}
	if runAt, err := time.Parse(time.RFC3339, t.Schedule.RunAt); err == nil {
		f.RunAt = runAt.In(time.Local).Format(localLayout)
	}
	if !knownAgent(t.Agent) {
		f.Agent, f.OtherAgent = otherAgent, t.Agent
	}
	if t.TimeoutMinutes > 0 {
		f.Timeout = strconv.Itoa(t.TimeoutMinutes)
	}
	if t.CatchUpMinutes != nil {
		f.CatchUp = strconv.Itoa(*t.CatchUpMinutes)
	}
	return f
}

func optionalInt(name, s string) (int, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", name)
	}
	return n, nil
}

func (f form) toTask() (task.Task, error) {
	t := task.Task{
		ID: strings.TrimSpace(f.ID), Title: strings.TrimSpace(f.Title), Prompt: f.Prompt, Cwd: strings.TrimSpace(f.Cwd),
		Enabled: f.Enabled, Agent: f.Agent, Model: strings.TrimSpace(f.Model), AgentArgs: strings.Fields(f.AgentArgs),
		Schedule: task.Schedule{Type: f.Trigger},
	}
	if t.Agent == otherAgent {
		t.Agent = strings.TrimSpace(f.OtherAgent)
	}
	switch f.Trigger {
	case "cron":
		t.Schedule.Expr = strings.TrimSpace(f.Cron)
	case "once":
		at, err := time.ParseInLocation(localLayout, strings.TrimSpace(f.RunAt), time.Local)
		if err != nil {
			return task.Task{}, fmt.Errorf("runAt must look like %s", localLayout)
		}
		t.Schedule.RunAt = at.Format(time.RFC3339)
	case "webhook":
		t.Schedule.Token = f.Token
		if t.Schedule.Token == "" {
			t.Schedule.Token = newToken()
		}
	}
	timeout, err := optionalInt("timeout", f.Timeout)
	if err != nil {
		return task.Task{}, err
	}
	t.TimeoutMinutes = timeout
	catchUp, err := optionalInt("catch-up", f.CatchUp)
	if err != nil {
		return task.Task{}, err
	}
	t.CatchUpMinutes = &catchUp
	return t, t.Validate()
}

func preview(s task.Schedule, now time.Time) string {
	if s.Type == "cron" {
		if err := task.ValidateCron(s.Expr); err != nil {
			return err.Error()
		}
	}
	var runs []string
	at := now
	for range 3 {
		n := schedule.Next(s, at)
		if n == nil {
			break
		}
		runs = append(runs, n.In(time.Local).Format("01-02 15:04"))
		at = *n
	}
	if len(runs) == 0 {
		return "no upcoming run"
	}
	return "next: " + strings.Join(runs, ", ")
}
