package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/app"
	"github.com/tiagokriok/herdr-triggers/internal/daemon"
	"github.com/tiagokriok/herdr-triggers/internal/runtime"
	"github.com/tiagokriok/herdr-triggers/internal/schedule"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/tui"
	"github.com/tiagokriok/herdr-triggers/internal/view"
	"github.com/tiagokriok/herdr-triggers/internal/webhook"
)

const usage = `herdr-triggers: scheduled and webhook-triggered agent runs for the Herdr app

  tui                    browse, add, edit and run tasks
  list [--json]          tasks with schedule, next and last run
  run-now <id> [--bg]    run a task now (--bg: return at once)
  enable <id> | disable <id>
  status [--json]        is the scheduler running
  start | stop           open or close the scheduler space in the app
  serve                  the scheduler itself (what start runs)
  token                  print a new random webhook token
`

const tick = 5 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func has(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func idArg(args []string) (string, error) {
	for _, a := range args {
		if len(a) > 0 && a[0] != '-' {
			return a, nil
		}
	}
	return "", errors.New("missing task id")
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		return nil
	}
	p, err := runtime.Paths()
	if err != nil {
		return err
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "token":
		b := make([]byte, 24)
		rand.Read(b)
		fmt.Println(hex.EncodeToString(b))
		return nil
	case "enable", "disable":
		id, err := idArg(rest)
		if err != nil {
			return err
		}
		if err := store.SetEnabled(p.Tasks, id, cmd == "enable"); err != nil {
			return err
		}
		fmt.Printf("%sd %s\n", cmd, id)
		return nil
	case "start", "stop", "status":
		d, err := runtime.DaemonDeps(p)
		if err != nil {
			return err
		}
		switch cmd {
		case "start":
			msg, err := daemon.Start(d)
			if err == nil {
				fmt.Println(msg)
			}
			return err
		case "stop":
			msg, err := daemon.Stop(d)
			if err == nil {
				fmt.Println(msg)
			}
			return err
		}
		s := daemon.Status(d)
		if has(rest, "--json") {
			return printJSON(s)
		}
		fmt.Println(s.State, s.Terminal)
		return nil
	}

	if cmd == "run-now" && has(rest, "--bg") {
		id, err := idArg(rest)
		if err != nil {
			return err
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		c := exec.Command(exe, "run-now", id)
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := c.Start(); err != nil {
			return err
		}
		fmt.Println("started", id)
		return c.Process.Release()
	}

	a, err := runtime.NewApp(p)
	if err != nil {
		return err
	}
	defer a.DB.Close()
	switch cmd {
	case "list":
		l := view.List(p.Tasks, a.DB, time.Now())
		if has(rest, "--json") {
			return printJSON(l)
		}
		if l.Error != "" {
			return errors.New(l.Error)
		}
		for _, t := range l.Tasks {
			next := "-"
			if t.Next != nil {
				next = t.Next.Local().Format("2006-01-02 15:04")
			}
			state := "on"
			if !t.Enabled {
				state = "off"
			}
			fmt.Printf("%-20s %-3s %-28s next %-16s %s\n", t.ID, state, t.Schedule, next, t.Agent)
		}
		return nil
	case "run-now":
		id, err := idArg(rest)
		if err != nil {
			return err
		}
		r, err := a.RunNow(id, "")
		if err != nil {
			return err
		}
		fmt.Printf("%s #%d %s\n", r.TaskID, r.N, r.Status)
		return nil
	case "serve":
		return serve(a)
	case "tui":
		return tui.Run(p, a)
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
}

func serve(a *app.App) error {
	a.Log("scheduler up, tasks: " + a.Tasks)
	a.Reconcile()
	srv := &http.Server{Addr: runtime.Addr(), Handler: webhook.Handler(a, func(f func()) { go f() }), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		a.Log("webhooks on http://" + srv.Addr + "/hooks/<id>")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.Log("webhook server: " + err.Error())
		}
	}()
	var state schedule.State
	for {
		next, _ := a.Step(state)
		for id, s := range next {
			if prev, ok := state[id]; !ok || !sameTime(prev.At, s.At) {
				at := "never"
				if s.At != nil {
					at = s.At.UTC().Format(time.RFC3339)
				}
				a.Log(id + " next: " + at)
			}
		}
		state = next
		time.Sleep(tick)
	}
}

func sameTime(a, b *time.Time) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(*b))
}
