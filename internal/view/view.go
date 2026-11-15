package view

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/schedule"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
)

var (
	days  = []string{"Sundays", "Mondays", "Tuesdays", "Wednesdays", "Thursdays", "Fridays", "Saturdays", "Sundays"}
	every = regexp.MustCompile(`^\*/(\d+)$`)
)

func isNum(s string) bool { _, err := strconv.Atoi(s); return err == nil }

func pad(s string) string { n, _ := strconv.Atoi(s); return fmt.Sprintf("%02d", n) }

func Describe(s task.Schedule) string {
	switch s.Type {
	case "webhook":
		return "on webhook"
	case "once":
		t, err := time.Parse(time.RFC3339, s.RunAt)
		if err != nil {
			return "once " + s.RunAt
		}
		return "once on " + t.In(time.Local).Format("2006-01-02 15:04")
	}
	f := strings.Fields(s.Expr)
	if len(f) != 5 {
		return "cron " + s.Expr
	}
	min, hour, dom, mon, dow := f[0], f[1], f[2], f[3], f[4]
	rest := hour == "*" && dom == "*" && mon == "*" && dow == "*"
	if m := every.FindStringSubmatch(min); m != nil && rest {
		return "every " + m[1] + " min"
	}
	if isNum(min) && rest {
		return "hourly at :" + pad(min)
	}
	if !isNum(min) || !isNum(hour) || mon != "*" {
		return "cron " + s.Expr
	}
	at := "at " + pad(hour) + ":" + pad(min)
	switch {
	case dom == "*" && dow == "*":
		return "daily " + at
	case dom == "*" && dow == "1-5":
		return "weekdays " + at
	case dom == "*" && isNum(dow):
		if n, _ := strconv.Atoi(dow); n <= 7 {
			return days[n] + " " + at
		}
	case isNum(dom) && dow == "*":
		n, _ := strconv.Atoi(dom)
		return fmt.Sprintf("monthly on day %d %s", n, at)
	}
	return "cron " + s.Expr
}

type Last struct {
	N        int       `json:"n"`
	Status   string    `json:"status"`
	FiredAt  time.Time `json:"firedAt"`
	Space    *string   `json:"space"`
	Terminal *string   `json:"terminal"`
}

type Task struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Schedule string     `json:"schedule"`
	Enabled  bool       `json:"enabled"`
	Agent    string     `json:"agent"`
	Model    string     `json:"model"`
	Next     *time.Time `json:"next"`
	Last     *Last      `json:"last"`
}

type Listing struct {
	Tasks       []Task `json:"tasks"`
	Error       string `json:"error,omitempty"`
	PublicURL   string `json:"publicUrl,omitempty"`
	PublicError string `json:"publicError,omitempty"`
	Scheduler   string `json:"scheduler"`
}

const pidKey = "scheduler_pid"

func MarkUp(db *store.DB, pid int) { db.SetMeta(pidKey, strconv.Itoa(pid)) }

func MarkDown(db *store.DB) { db.SetMeta(pidKey, "") }

func Scheduler(db *store.DB, alive func(int) bool) string {
	pid, err := strconv.Atoi(db.Meta(pidKey))
	if err != nil || !alive(pid) {
		return "stopped"
	}
	return "live"
}

const (
	PublicURLKey   = "public_url"
	PublicErrorKey = "public_error"
)

func HookURL(base, id, token string) string {
	return base + "/hooks/" + id + "/" + token
}

func orNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func List(tasksFile string, db *store.DB, now time.Time) Listing {
	f, err := store.LoadTasks(tasksFile)
	if err != nil {
		return Listing{Tasks: []Task{}, Error: err.Error()}
	}
	out := Listing{Tasks: []Task{}, PublicURL: db.Meta(PublicURLKey), PublicError: db.Meta(PublicErrorKey), Scheduler: Scheduler(db, IsScheduler)}
	for _, t := range f.Tasks {
		v := Task{ID: t.ID, Title: t.Title, Schedule: Describe(t.Schedule), Enabled: t.Enabled, Agent: t.Agent, Model: t.Model}
		if t.Enabled {
			if n := schedule.Next(t.Schedule, now); n != nil {
				utc := n.UTC()
				v.Next = &utc
			}
		}
		if last, _ := db.Last(t.ID); last != nil {
			v.Last = &Last{N: last.N, Status: last.Status, FiredAt: last.FiredAt, Space: orNil(last.Space), Terminal: orNil(last.Terminal)}
		}
		out.Tasks = append(out.Tasks, v)
	}
	return out
}
