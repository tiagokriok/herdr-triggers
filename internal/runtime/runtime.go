package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/app"
	"github.com/tiagokriok/herdr-triggers/internal/daemon"
	"github.com/tiagokriok/herdr-triggers/internal/dispatch"
	"github.com/tiagokriok/herdr-triggers/internal/store"
)

func Exec(argv []string) dispatch.Result {
	cmd := exec.Command(argv[0], argv[1:]...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		code = 127
		errb.WriteString(err.Error())
	}
	return dispatch.Result{Code: code, Stdout: out.String(), Stderr: errb.String()}
}

func notifyArgv(title, body string) []string {
	if goruntime.GOOS == "darwin" {
		return []string{"osascript", "-e", fmt.Sprintf("display notification %q with title %q", body, title)}
	}
	return []string{"notify-send", "--app-name=Triggers", title, body}
}

func Notify(title, body string) { go Exec(notifyArgv(title, body)) }

func Log(msg string) { fmt.Printf("%s %s\n", time.Now().UTC().Format(time.RFC3339), msg) }

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func HC() string   { return env("TRIGGERS_HC", "herdr-core") }
func App() string  { return env("TRIGGERS_APP", "herdr-app") }
func Addr() string { return env("TRIGGERS_ADDR", "127.0.0.1:8787") }

func Paths() (store.Paths, error) { return store.ResolvePaths(os.Getenv) }

func NewApp(p store.Paths) (*app.App, error) {
	db, err := store.Open(p.DB)
	if err != nil {
		return nil, err
	}
	return &app.App{
		Tasks: p.Tasks, DB: db, Log: Log, Notify: Notify,
		Dispatch: dispatch.Deps{
			Exec: Exec, HC: HC(), App: App(), Now: time.Now, Pause: time.Sleep,
			ConfigDir: os.Getenv("CLAUDE_CONFIG_DIR"),
		},
	}, nil
}

func DaemonDeps(p store.Paths) (daemon.Deps, error) {
	exe, err := os.Executable()
	if err != nil {
		return daemon.Deps{}, err
	}
	return daemon.Deps{
		Exec: Exec, HC: HC(), App: App(), Exe: exe,
		File: p.Scheduler, CLIFile: p.CLI, ConfigDir: os.Getenv("CLAUDE_CONFIG_DIR"),
	}, nil
}
