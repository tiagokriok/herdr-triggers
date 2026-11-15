package dispatch

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/agents"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
)

type Result struct {
	Code   int
	Stdout string
	Stderr string
}

type Exec func(argv []string) Result

type Deps struct {
	Exec      Exec
	HC        string
	App       string
	OnUpdate  func(store.Run)
	Now       func() time.Time
	Pause     func(time.Duration)
	ConfigDir string
}

const (
	waitSeconds     = "30"
	screenTailLines = 5
	startupPause    = 2 * time.Second
)

var (
	promptOnScreen = regexp.MustCompile(`Enter to confirm|Do you want to proceed\?|Esc to cancel`)
	checkIns       = []string{"timeout", "message"}
)

func ParseJSON[T any](s string) (T, bool) {
	var v T
	return v, json.Unmarshal([]byte(s), &v) == nil
}

func output(r Result) string {
	if s := strings.TrimSpace(r.Stderr); s != "" {
		return s
	}
	return strings.TrimSpace(r.Stdout)
}

func finish(r store.Run, d Deps, status, errText string) store.Run {
	r.Status = status
	if errText != "" {
		r.Error = errText
	}
	d.OnUpdate(r)
	return r
}

type agentStatus struct {
	State  string `json:"state"`
	Status *struct {
		State string `json:"state"`
	} `json:"status"`
}

func isBlocked(d Deps, terminal string) bool {
	got, _ := ParseJSON[agentStatus](d.Exec([]string{d.HC, "get", "--json", terminal}).Stdout)
	if got.Status != nil {
		switch got.Status.State {
		case "blocked":
			return true
		case "working":
			return false
		}
	}
	screen := d.Exec([]string{d.HC, "read", terminal})
	if screen.Code != 0 {
		return false
	}
	lines := strings.Split(strings.TrimRight(screen.Stdout, " \n\t"), "\n")
	return promptOnScreen.MatchString(strings.Join(lines[max(0, len(lines)-screenTailLines):], "\n"))
}

type waitResult struct {
	Ended  bool    `json:"ended"`
	Reason *string `json:"reason"`
	Status *string `json:"status"`
	Turn   *int    `json:"turn"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func WatchRun(r store.Run, d Deps) store.Run {
	for {
		waited := d.Exec([]string{d.HC, "wait", "--json", "--timeout", waitSeconds, r.Terminal})
		res, ok := ParseJSON[*waitResult](waited.Stdout)
		if !ok || res == nil {
			return finish(r, d, "failed", "hc wait: "+output(waited))
		}
		reason, status := deref(res.Reason), deref(res.Status)
		startupIdle := reason == "status" && status == "idle" && res.Turn != nil && *res.Turn == 0
		if !startupIdle && (res.Ended || (reason != "" && !slices.Contains(checkIns, reason))) {
			end := reason
			if reason == "status" {
				end = status
			}
			if end == "" {
				end = "done"
			}
			return finish(r, d, end, "")
		}
		if r.TimeoutAt != nil && d.Now().After(*r.TimeoutAt) {
			d.Exec([]string{d.HC, "kill", r.Terminal})
			return finish(r, d, "timeout", "")
		}
		if startupIdle {
			d.Pause(startupPause)
			continue
		}
		blocked := isBlocked(d, r.Terminal)
		if blocked != (r.Status == "blocked") {
			next := "working"
			if blocked {
				next = "blocked"
			}
			r = finish(r, d, next, "")
		}
	}
}

type spaceMade struct {
	Space    string `json:"space"`
	Terminal string `json:"terminal"`
	Error    string `json:"error"`
}

func RunTask(t task.Task, r store.Run, d Deps) store.Run {
	payload, _ := json.Marshal(map[string]any{
		"name":   fmt.Sprintf("%s run %d", t.Title, r.N),
		"folder": t.Cwd,
		"argv":   agents.Argv(t, d.ConfigDir),
		"task":   t.Prompt,
	})
	started := d.Exec([]string{d.App, "do", "space.new", string(payload)})
	made, ok := ParseJSON[spaceMade](started.Stdout)
	if started.Code != 0 || !ok || made.Terminal == "" {
		msg := made.Error
		if msg == "" {
			msg = output(started)
		}
		return finish(r, d, "failed", "space.new: "+msg)
	}
	r.Terminal, r.Space = made.Terminal, made.Space
	if t.TimeoutMinutes > 0 {
		limit := d.Now().Add(time.Duration(t.TimeoutMinutes) * time.Minute)
		r.TimeoutAt = &limit
	}
	r = finish(r, d, "working", "")
	return WatchRun(r, d)
}
