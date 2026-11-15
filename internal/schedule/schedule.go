package schedule

import (
	"encoding/json"
	"time"

	"github.com/adhocore/gronx"
	"github.com/tiagokriok/herdr-triggers/internal/task"
)

const Grace = time.Minute

func Next(s task.Schedule, after time.Time) *time.Time {
	switch s.Type {
	case "once":
		runAt, err := time.Parse(time.RFC3339, s.RunAt)
		if err != nil || !runAt.After(after) {
			return nil
		}
		return &runAt
	case "cron":
		next, err := gronx.NextTickAfter(s.Expr, after.In(time.Local), false)
		if err != nil {
			return nil
		}
		return &next
	}
	return nil
}

type Armed struct {
	Sig string
	At  *time.Time
}

type State map[string]Armed

type Missed struct {
	Task task.Task
	At   time.Time
}

type TickPlan struct {
	Fire   []task.Task
	Missed []Missed
	State  State
}

func signature(t task.Task) string {
	b, _ := json.Marshal(t.Schedule)
	return string(b)
}

func window(t task.Task) time.Duration {
	return max(Grace, t.CatchUp())
}

func Plan(tasks []task.Task, prev State, now time.Time) TickPlan {
	p := TickPlan{State: State{}}
	for _, t := range tasks {
		if !t.Enabled {
			continue
		}
		sig := signature(t)
		armed, ok := prev[t.ID]
		if !ok || armed.Sig != sig {
			if t.Schedule.Type == "once" {
				if runAt, err := time.Parse(time.RFC3339, t.Schedule.RunAt); err == nil && !runAt.After(now) {
					if now.Sub(runAt) <= window(t) {
						p.Fire = append(p.Fire, t)
					} else {
						p.Missed = append(p.Missed, Missed{t, runAt})
					}
				}
			}
			p.State[t.ID] = Armed{sig, Next(t.Schedule, now)}
			continue
		}
		if armed.At == nil || armed.At.After(now) {
			p.State[t.ID] = armed
			continue
		}
		if now.Sub(*armed.At) <= window(t) {
			p.Fire = append(p.Fire, t)
		} else {
			p.Missed = append(p.Missed, Missed{t, *armed.At})
		}
		p.State[t.ID] = Armed{sig, Next(t.Schedule, now)}
	}
	return p
}
