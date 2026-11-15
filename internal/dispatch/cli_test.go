package dispatch

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/store"
)

type cliFake struct {
	calls   [][]string
	spawned [][]string
	answers map[string][]Result
}

func errJSON(code string) Result {
	b, _ := json.Marshal(map[string]any{"error": map[string]string{"code": code, "message": code + " happened"}})
	return Result{Code: 1, Stderr: string(b)}
}

func agentJSON(status string) Result {
	return js(map[string]any{"result": map[string]any{"agent": map[string]any{"agent_status": status, "name": "t-nightly-3"}}})
}

var wsCreated = js(map[string]any{"result": map[string]any{"workspace": map[string]any{"workspace_id": "w4"}, "root_pane": map[string]any{"pane_id": "w4:p1"}}})

func (f *cliFake) exec(argv []string) Result {
	f.calls = append(f.calls, argv)
	key := argv[1]
	if len(argv) > 2 && !strings.HasPrefix(argv[2], "-") {
		key += " " + argv[2]
	}
	if q := f.answers[key]; len(q) > 0 {
		f.answers[key] = q[1:]
		return q[0]
	}
	switch key {
	case "workspace list":
		return js(map[string]any{"result": map[string]any{"workspaces": []any{}}})
	case "workspace create":
		return wsCreated
	case "agent start":
		return agentJSON("idle")
	case "agent wait":
		if slices.Contains(argv, "working") && !slices.Contains(argv, "done") {
			return agentJSON("working")
		}
		return agentJSON("done")
	}
	return js(map[string]any{})
}

func cliDeps(f *cliFake, seen *[]string) Deps {
	return Deps{
		Backend: "cli", Herdr: []string{"herdr"}, Exec: f.exec,
		OnUpdate: func(r store.Run) {
			if seen != nil {
				*seen = append(*seen, r.Status)
			}
		},
		Now:   func() time.Time { return mustTime(now) },
		Pause: func(time.Duration) {},
		Spawn: func(argv []string) error { f.spawned = append(f.spawned, argv); return nil },
	}
}

func find(calls [][]string, prefix ...string) []string {
	for _, c := range calls {
		if len(c) >= len(prefix) && slices.Equal(c[:len(prefix)], prefix) {
			return c
		}
	}
	return nil
}

func TestCLIRun(t *testing.T) {
	f := &cliFake{answers: map[string][]Result{}}
	tk := base
	tk.Model = "opus"
	tk.AgentArgs = []string{"--verbose"}
	var seen []string
	d := cliDeps(f, &seen)
	d.ConfigDir = "/cfg"
	final := RunTask(tk, rec(), d)
	if final.Status != "done" || final.Backend != "cli" || final.Terminal != "t-nightly-3" || final.Space != "w4" || !slices.Equal(seen, []string{"starting", "working", "done"}) {
		t.Fatalf("%+v %v", final, seen)
	}
	wants := [][]string{
		{"herdr", "workspace", "create", "--cwd", "/w/proj", "--label", "Nightly triage run 3", "--no-focus", "--env", "CLAUDE_CONFIG_DIR=/cfg"},
		{"herdr", "agent", "start", "t-nightly-3", "--kind", "claude", "--pane", "w4:p1", "--timeout", "60000", "--", "--model", "opus", "--verbose", "--", "triage the inbox"},
	}
	for _, want := range wants {
		if got := find(f.calls, want[:3]...); !slices.Equal(got, want) {
			t.Errorf("got %q\nwant %q", got, want)
		}
	}
	if len(f.spawned) != 0 {
		t.Fatal(f.spawned)
	}
}

func TestCLIStartsServer(t *testing.T) {
	f := &cliFake{answers: map[string][]Result{"workspace list": {errJSON("unavailable"), errJSON("unavailable")}}}
	RunTask(base, rec(), cliDeps(f, nil))
	if len(f.spawned) != 1 || !slices.Equal(f.spawned[0], []string{"herdr", "server"}) {
		t.Fatal(f.spawned)
	}
}

func TestCLISessionPrefix(t *testing.T) {
	f := &cliFake{answers: map[string][]Result{}}
	d := cliDeps(f, nil)
	d.Herdr = []string{"herdr", "--session", "hooks"}
	f2 := func(argv []string) Result { return f.exec(append([]string{argv[0]}, argv[3:]...)) }
	d.Exec = func(argv []string) Result {
		if !slices.Equal(argv[:3], []string{"herdr", "--session", "hooks"}) {
			t.Fatalf("%q", argv)
		}
		return f2(argv)
	}
	if final := RunTask(base, rec(), d); final.Status != "done" {
		t.Fatalf("%+v", final)
	}
}

func TestCLIWatch(t *testing.T) {
	cases := []struct {
		name    string
		start   Result
		waits   []Result
		final   string
		seen    []string
		timeout bool
	}{
		{"false ready, then works", agentJSON("idle"), []Result{errJSON("timeout"), agentJSON("working"), agentJSON("idle")}, "done", []string{"starting", "working", "done"}, false},
		{"already working", agentJSON("working"), []Result{agentJSON("done")}, "done", []string{"working", "done"}, false},
		{"blocked then done", agentJSON("idle"), []Result{agentJSON("blocked"), agentJSON("working"), agentJSON("done")}, "done", []string{"starting", "blocked", "working", "done"}, false},
		{"agent gone", agentJSON("idle"), []Result{errJSON("agent_not_found")}, "exit", []string{"starting", "exit"}, false},
		{"timeout kills", agentJSON("working"), []Result{errJSON("timeout"), errJSON("timeout")}, "timeout", []string{"working", "timeout"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &cliFake{answers: map[string][]Result{"agent start": {c.start}, "agent wait": c.waits}}
			var seen []string
			tk := base
			d := cliDeps(f, &seen)
			if c.timeout {
				tk.TimeoutMinutes = 1
				calls := 0
				d.Now = func() time.Time { calls++; return mustTime(now).Add(time.Duration(calls) * time.Minute) }
			}
			final := RunTask(tk, rec(), d)
			if final.Status != c.final || !slices.Equal(seen, c.seen) {
				t.Fatalf("%+v %v", final, seen)
			}
			closed := find(f.calls, "herdr", "workspace", "close")
			if c.timeout != (closed != nil) || (closed != nil && closed[3] != "w4") {
				t.Fatalf("close %q", closed)
			}
		})
	}
}

func TestCLINeverStarts(t *testing.T) {
	f := &cliFake{answers: map[string][]Result{"agent wait": {errJSON("timeout"), errJSON("timeout"), errJSON("timeout")}}}
	d := cliDeps(f, nil)
	calls := 0
	d.Now = func() time.Time { calls++; return mustTime(now).Add(time.Duration(calls) * 10 * time.Minute) }
	final := RunTask(base, rec(), d)
	if final.Status != "failed" || !strings.Contains(final.Error, "never started") {
		t.Fatalf("%+v", final)
	}
}

func TestCLIPromptArgv(t *testing.T) {
	f := &cliFake{answers: map[string][]Result{}}
	tk := base
	tk.Agent = "opencode"
	RunTask(tk, rec(), cliDeps(f, nil))
	got := find(f.calls, "herdr", "agent", "start")
	if !slices.Equal(got[len(got)-3:], []string{"--", "--prompt", "triage the inbox"}) {
		t.Fatalf("%q", got)
	}
}

func TestCLIStartFailures(t *testing.T) {
	f := &cliFake{answers: map[string][]Result{"agent start": {errJSON("agent_not_ready")}}}
	final := RunTask(base, rec(), cliDeps(f, nil))
	if final.Status != "failed" || !strings.Contains(final.Error, "agent_not_ready") || find(f.calls, "herdr", "agent", "wait") != nil {
		t.Fatalf("%+v", final)
	}
	f = &cliFake{answers: map[string][]Result{"workspace create": {errJSON("invalid_cwd")}}}
	if final := RunTask(base, rec(), cliDeps(f, nil)); final.Status != "failed" || !strings.Contains(final.Error, "invalid_cwd") {
		t.Fatalf("%+v", final)
	}
}

func TestAgentName(t *testing.T) {
	for in, want := range map[string]string{"nightly": "t-nightly-3", "9lives": "t-9lives-3", strings.Repeat("a", 40): "t-" + strings.Repeat("a", 28) + "-3"} {
		if got := agentName(in, 3); got != want || len(got) > 32 {
			t.Errorf("%s: %s", in, got)
		}
	}
}

func TestAlive(t *testing.T) {
	f := &cliFake{answers: map[string][]Result{"agent get": {agentJSON("working"), errJSON("agent_not_found")}}}
	d := cliDeps(f, nil)
	r := rec()
	r.Backend, r.Terminal = "cli", "t-nightly-3"
	if !Alive(r, d) || Alive(r, d) {
		t.Fatal(f.calls)
	}
}

func TestCLIFlattensPromptAndRetriesBusyPane(t *testing.T) {
	f := &cliFake{answers: map[string][]Result{"agent start": {errJSON("agent_pane_busy"), errJSON("agent_pane_busy"), agentJSON("working")}}}
	tk := base
	tk.Prompt = "line one\r\nline two\nend"
	final := RunTask(tk, rec(), cliDeps(f, nil))
	starts := 0
	for _, c := range f.calls {
		if len(c) > 2 && c[1] == "agent" && c[2] == "start" {
			starts++
			if got := c[len(c)-1]; got != `line one\nline two\nend` {
				t.Fatalf("%q", got)
			}
		}
	}
	if starts != 3 || final.Status != "done" {
		t.Fatalf("%d %+v", starts, final)
	}
}

func seqJSON(status string, seq int) Result {
	return js(map[string]any{"result": map[string]any{"agent": map[string]any{"agent_status": status, "state_change_seq": seq}}})
}

func TestCLIFastTurnSeenBySeq(t *testing.T) {
	f := &cliFake{answers: map[string][]Result{
		"agent start": {seqJSON("idle", 3)},
		"agent wait":  {errJSON("timeout"), errJSON("timeout")},
		"agent get":   {seqJSON("idle", 4), seqJSON("idle", 5)},
	}}
	var seen []string
	final := RunTask(base, rec(), cliDeps(f, &seen))
	if final.Status != "done" || !slices.Equal(seen, []string{"starting", "done"}) {
		t.Fatalf("%+v %v", final, seen)
	}
}
