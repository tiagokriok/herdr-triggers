package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tiagokriok/herdr-triggers/internal/dispatch"
)

type env struct {
	d     Deps
	calls [][]string
	gets  map[string]string
	made  dispatch.Result
}

func newEnv(t *testing.T) *env {
	dir := t.TempDir()
	e := &env{gets: map[string]string{}, made: dispatch.Result{Stdout: `{"space":"sp1","tab":"tb1","terminal":"t7"}`}}
	e.d = Deps{
		HC: "hc", App: "herdr-app", Exe: "/opt/bin/herdr-triggers",
		File: filepath.Join(dir, "scheduler.json"), CLIFile: filepath.Join(dir, "cli.json"),
		Exec: func(argv []string) dispatch.Result {
			e.calls = append(e.calls, argv)
			if argv[0] == "herdr-app" && argv[2] == "space.new" {
				return e.made
			}
			if argv[1] == "get" {
				if s, ok := e.gets[argv[len(argv)-1]]; ok {
					return dispatch.Result{Stdout: s}
				}
				return dispatch.Result{Code: 1, Stderr: "not_found"}
			}
			return dispatch.Result{Stdout: "{}"}
		},
	}
	return e
}

const running = `{"state":"live","foreground":{"argv":["/opt/bin/herdr-triggers","serve"]}}`
const shell = `{"state":"live","foreground":{"argv":["/usr/bin/zsh"]}}`

func (e *env) remember() { os.WriteFile(e.d.File, []byte(`{"space":"sp0","terminal":"t0"}`), 0o644) }

func has(calls [][]string, want ...string) bool {
	return slices.ContainsFunc(calls, func(c []string) bool { return slices.Equal(c, want) })
}

func TestStart(t *testing.T) {
	t.Run("opens a Triggers space and remembers it", func(t *testing.T) {
		e := newEnv(t)
		e.d.ConfigDir = "/cfg"
		msg, err := Start(e.d)
		payload, _ := json.Marshal(map[string]any{"name": "Triggers", "folder": "/opt/bin", "argv": []string{"env", "CLAUDE_CONFIG_DIR=/cfg", "/opt/bin/herdr-triggers", "serve"}})
		if err != nil || msg != "started t7" || !has(e.calls, "herdr-app", "do", "space.new", string(payload)) {
			t.Fatal(msg, err, e.calls)
		}
		saved, _ := os.ReadFile(e.d.File)
		cli, _ := os.ReadFile(e.d.CLIFile)
		if !strings.Contains(string(saved), `"terminal": "t7"`) || !strings.Contains(string(cli), `"/opt/bin/herdr-triggers"`) {
			t.Fatal(string(saved), string(cli))
		}
	})
	t.Run("idempotent while running", func(t *testing.T) {
		e := newEnv(t)
		e.remember()
		e.gets["t0"] = running
		if msg, _ := Start(e.d); msg != "already running t0" || len(e.calls) != 1 {
			t.Fatal(msg, e.calls)
		}
	})
	t.Run("replaces a shell-only space", func(t *testing.T) {
		e := newEnv(t)
		e.remember()
		e.gets["t0"] = shell
		msg, _ := Start(e.d)
		if msg != "started t7" || !has(e.calls, "herdr-app", "do", "space.close", "--id", "sp0", "--confirmed", "true") {
			t.Fatal(msg, e.calls)
		}
	})
	t.Run("fails loudly", func(t *testing.T) {
		e := newEnv(t)
		e.made = dispatch.Result{Code: 1, Stdout: `{"error":"no window"}`}
		if _, err := Start(e.d); err == nil || !strings.Contains(err.Error(), "no window") {
			t.Fatal(err)
		}
	})
}

func TestStop(t *testing.T) {
	e := newEnv(t)
	if msg, _ := Stop(e.d); msg != "not running" {
		t.Fatal(msg)
	}
	e.remember()
	msg, _ := Stop(e.d)
	if _, err := os.Stat(e.d.File); msg != "stopped" || err == nil || !has(e.calls, "herdr-app", "do", "space.close", "--id", "sp0", "--confirmed", "true") {
		t.Fatal(msg, e.calls)
	}
}

func TestStatus(t *testing.T) {
	e := newEnv(t)
	if s := Status(e.d); s.State != "stopped" {
		t.Fatal(s)
	}
	e.remember()
	if s := Status(e.d); s.State != "stopped" {
		t.Fatal(s)
	}
	e.gets["t0"] = running
	if s := Status(e.d); s != (State{State: "live", Space: "sp0", Terminal: "t0"}) {
		t.Fatal(s)
	}
}
