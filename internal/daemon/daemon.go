package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tiagokriok/herdr-triggers/internal/agents"
	"github.com/tiagokriok/herdr-triggers/internal/dispatch"
	"github.com/tiagokriok/herdr-triggers/internal/store"
)

type Deps struct {
	Exec      dispatch.Exec
	HC        string
	App       string
	Exe       string
	File      string
	CLIFile   string
	ConfigDir string
}

type State struct {
	State    string `json:"state"`
	Space    string `json:"space,omitempty"`
	Terminal string `json:"terminal,omitempty"`
}

type ref struct {
	Space    string `json:"space"`
	Terminal string `json:"terminal"`
	Error    string `json:"error,omitempty"`
}

const (
	SpaceName = "Triggers"
	serveArg  = "serve"
)

func remembered(file string) (ref, bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		return ref{}, false
	}
	r, ok := dispatch.ParseJSON[ref](string(data))
	return r, ok && r.Terminal != ""
}

func runsScheduler(d Deps, terminal string) bool {
	got := d.Exec([]string{d.HC, "get", "--json", terminal})
	if got.Code != 0 {
		return false
	}
	t, ok := dispatch.ParseJSON[struct {
		State      string `json:"state"`
		Foreground struct {
			Argv []string `json:"argv"`
		} `json:"foreground"`
	}](got.Stdout)
	argv := t.Foreground.Argv
	return ok && t.State == "live" && slices.Contains(argv, serveArg) &&
		slices.ContainsFunc(argv, func(a string) bool { return filepath.Base(a) == filepath.Base(d.Exe) })
}

func closeSpace(d Deps, space string) {
	d.Exec([]string{d.App, "do", "space.close", "--id", space, "--confirmed", "true"})
}

func WriteCLI(d Deps) error {
	return store.WriteJSON(d.CLIFile, map[string]any{"argv": []string{d.Exe}})
}

func Start(d Deps) (string, error) {
	if err := WriteCLI(d); err != nil {
		return "", err
	}
	if prev, ok := remembered(d.File); ok {
		if runsScheduler(d, prev.Terminal) {
			return "already running " + prev.Terminal, nil
		}
		closeSpace(d, prev.Space)
	}
	payload, _ := json.Marshal(map[string]any{
		"name":   SpaceName,
		"folder": filepath.Dir(d.Exe),
		"argv":   agents.EnvArgv(d.ConfigDir, []string{d.Exe, serveArg}),
	})
	made := d.Exec([]string{d.App, "do", "space.new", string(payload)})
	r, ok := dispatch.ParseJSON[ref](made.Stdout)
	if made.Code != 0 || !ok || r.Space == "" || r.Terminal == "" {
		msg := r.Error
		if msg == "" {
			msg = strings.TrimSpace(made.Stderr + made.Stdout)
		}
		return "", errors.New("space.new failed: " + msg)
	}
	if err := store.WriteJSON(d.File, ref{Space: r.Space, Terminal: r.Terminal}); err != nil {
		return "", err
	}
	return fmt.Sprintf("started %s", r.Terminal), nil
}

func Stop(d Deps) (string, error) {
	prev, ok := remembered(d.File)
	if !ok {
		return "not running", nil
	}
	closeSpace(d, prev.Space)
	os.Remove(d.File)
	return "stopped", nil
}

func Status(d Deps) State {
	prev, ok := remembered(d.File)
	if ok && runsScheduler(d, prev.Terminal) {
		return State{State: "live", Space: prev.Space, Terminal: prev.Terminal}
	}
	return State{State: "stopped"}
}
