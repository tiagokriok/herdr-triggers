package dispatch

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/agents"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
)

const (
	cliWaitMS    = "30000"
	agentStartMS = "60000"
	serverTries  = 30
	maxAgentName = 32
	startLimit   = 15 * time.Minute
	paneTries    = 30
)

var nameUnsafe = regexp.MustCompile(`[^a-z0-9_-]`)

type cliReply struct {
	Result struct {
		Agent struct {
			Status string `json:"agent_status"`
			Seq    int    `json:"state_change_seq"`
		} `json:"agent"`
		Workspace struct {
			ID string `json:"workspace_id"`
		} `json:"workspace"`
		RootPane struct {
			ID string `json:"pane_id"`
		} `json:"root_pane"`
	} `json:"result"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (d Deps) herdr(args ...string) []string {
	return append(append([]string{}, d.Herdr...), args...)
}

func cliCall(d Deps, args ...string) (cliReply, string) {
	res := d.Exec(d.herdr(args...))
	reply, ok := ParseJSON[cliReply](res.Stdout)
	if res.Code == 0 && ok {
		return reply, ""
	}
	if failed, ok := ParseJSON[cliReply](res.Stderr); ok && failed.Error != nil {
		return failed, failed.Error.Code
	}
	return reply, "error: " + output(res)
}

func errorText(reply cliReply, code string) string {
	if reply.Error != nil && reply.Error.Message != "" {
		return code + ": " + reply.Error.Message
	}
	return code
}

var newlines = strings.NewReplacer("\r\n", `\n`, "\n", `\n`, "\r", `\n`)

func singleLine(s string) string { return newlines.Replace(s) }

func agentName(id string, n int) string {
	suffix := fmt.Sprintf("-%d", n)
	name := "t-" + nameUnsafe.ReplaceAllString(id, "-")
	return name[:min(len(name), maxAgentName-len(suffix))] + suffix
}

func ensureServer(d Deps) error {
	if _, code := cliCall(d, "workspace", "list"); code == "" {
		return nil
	}
	if err := d.Spawn(d.herdr("server")); err != nil {
		return err
	}
	var code string
	for range serverTries {
		d.Pause(500 * time.Millisecond)
		if _, code = cliCall(d, "workspace", "list"); code == "" {
			return nil
		}
	}
	return fmt.Errorf("herdr server did not start: %s", code)
}

func runCLI(t task.Task, r store.Run, d Deps) store.Run {
	r.Backend = "cli"
	if err := ensureServer(d); err != nil {
		return finish(r, d, "failed", err.Error())
	}
	create := []string{"workspace", "create", "--cwd", t.Cwd, "--label", fmt.Sprintf("%s run %d", t.Title, r.N), "--no-focus"}
	configDir := d.ConfigDir
	if configDir == "" {
		configDir = t.ClaudeConfigDir
	}
	if t.Agent == "claude" && configDir != "" {
		create = append(create, "--env", "CLAUDE_CONFIG_DIR="+configDir)
	}
	ws, code := cliCall(d, create...)
	if code != "" {
		return finish(r, d, "failed", "workspace create: "+errorText(ws, code))
	}
	r.Space = ws.Result.Workspace.ID
	name := agentName(t.ID, r.N)
	plain := t
	plain.ClaudeConfigDir = ""
	args := append(agents.Argv(plain, "")[1:], singleLine(t.Prompt))
	start := append([]string{"agent", "start", name, "--kind", t.Agent, "--pane", ws.Result.RootPane.ID, "--timeout", agentStartMS, "--"}, args...)
	reply, code := cliCall(d, start...)
	for try := 0; code == "agent_pane_busy" && try < paneTries; try++ {
		d.Pause(500 * time.Millisecond)
		reply, code = cliCall(d, start...)
	}
	if code != "" {
		return finish(r, d, "failed", "agent start: "+errorText(reply, code))
	}
	r.Terminal = name
	if t.TimeoutMinutes > 0 {
		limit := d.Now().Add(time.Duration(t.TimeoutMinutes) * time.Minute)
		r.TimeoutAt = &limit
	}
	next := "starting"
	if st := reply.Result.Agent.Status; st == "working" || st == "blocked" {
		next = st
	}
	r = finish(r, d, next, "")
	return watchCLI(r, d, reply.Result.Agent.Seq)
}

func cliState(status string) (string, bool) {
	switch status {
	case "idle", "done":
		return "done", true
	case "blocked", "working":
		return status, false
	}
	return "", false
}

func untilFor(status string) []string {
	var states []string
	switch status {
	case "starting":
		states = []string{"working", "blocked"}
	case "blocked":
		states = []string{"done", "idle", "working"}
	default:
		states = []string{"done", "idle", "blocked"}
	}
	var args []string
	for _, s := range states {
		args = append(args, "--until", s)
	}
	return args
}

func turnPassed(r store.Run, d Deps, startSeq int) bool {
	if startSeq < 0 {
		return false
	}
	got, code := cliCall(d, "agent", "get", r.Terminal)
	_, end := cliState(got.Result.Agent.Status)
	return code == "" && end && got.Result.Agent.Seq >= startSeq+2
}

func watchCLI(r store.Run, d Deps, startSeq int) store.Run {
	for {
		args := append([]string{"agent", "wait", r.Terminal}, untilFor(r.Status)...)
		reply, code := cliCall(d, append(args, "--timeout", cliWaitMS)...)
		switch {
		case code == "":
			next, end := cliState(reply.Result.Agent.Status)
			if end && r.Status != "starting" {
				return finish(r, d, next, "")
			}
			if next != "" && next != r.Status && !end {
				r = finish(r, d, next, "")
			}
		case code != "timeout":
			return finish(r, d, "exit", strings.TrimPrefix(errorText(reply, code), "error: "))
		case r.Status == "starting" && turnPassed(r, d, startSeq):
			return finish(r, d, "done", "")
		}
		if r.TimeoutAt != nil && d.Now().After(*r.TimeoutAt) {
			if r.Space != "" {
				d.Exec(d.herdr("workspace", "close", r.Space))
			}
			return finish(r, d, "timeout", "")
		}
		if r.Status == "starting" && d.Now().Sub(r.FiredAt) > startLimit {
			return finish(r, d, "failed", "agent never started working")
		}
	}
}
