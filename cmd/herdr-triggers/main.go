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
	"os/signal"
	"syscall"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/app"
	"github.com/tiagokriok/herdr-triggers/internal/config"
	"github.com/tiagokriok/herdr-triggers/internal/daemon"
	"github.com/tiagokriok/herdr-triggers/internal/runtime"
	"github.com/tiagokriok/herdr-triggers/internal/schedule"
	"github.com/tiagokriok/herdr-triggers/internal/service"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/tui"
	"github.com/tiagokriok/herdr-triggers/internal/tunnel"
	"github.com/tiagokriok/herdr-triggers/internal/view"
	"github.com/tiagokriok/herdr-triggers/internal/webhook"
)

const usage = `herdr-triggers: scheduled and webhook-triggered agent runs for the Herdr app

  tui                    browse, add, edit and run tasks
  list [--json]          tasks with schedule, next and last run
  run-now <id> [--bg]    run a task now (--bg: return at once)
  enable <id> | disable <id>
  status [--json]        is the scheduler running, which backend runs agents
  service install | uninstall | restart
                         run the scheduler as a user service (systemd, launchd):
                         starts on login, survives closing the app; for servers
  start | stop           or run the scheduler in a space of the Herdr app
  serve                  the scheduler itself (what start runs)
  token                  print a new random webhook token
  public                 show how webhooks are exposed and the public url
  public <provider> [x]  none | url <baseUrl> | tailscale | ngrok [domain] | cloudflared
  backend [auto|app|cli] [session]
                         where runs open: the Herdr app, or the herdr CLI server
                         (auto: the app when installed); restart to apply
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
	case "public":
		if len(rest) == 0 {
			break
		}
		cfg, err := config.Load(p.Config)
		if err != nil {
			return err
		}
		cfg.Public = tunnel.Config{Provider: rest[0]}
		if len(rest) > 1 {
			if rest[0] == "url" {
				cfg.Public.BaseURL = rest[1]
			} else {
				cfg.Public.Domain = rest[1]
			}
		}
		if err := config.Save(p.Config, cfg); err != nil {
			return err
		}
		fmt.Println("saved; restart the scheduler to apply")
		return nil
	case "backend":
		cfg, err := config.Load(p.Config)
		if err != nil {
			return err
		}
		if len(rest) == 0 {
			fmt.Printf("backend %s (runs use %s)\n", cfg.Backend, cfg.ResolveBackend(exec.LookPath))
			return nil
		}
		cfg.Backend, cfg.Session = rest[0], ""
		if len(rest) > 1 {
			cfg.Session = rest[1]
		}
		if err := config.Save(p.Config, cfg); err != nil {
			return err
		}
		fmt.Println("saved; restart the scheduler to apply")
		return nil
	case "service":
		d, err := runtime.ServiceDeps()
		if err != nil {
			return err
		}
		switch {
		case has(rest, "install"):
			dd, err := runtime.DaemonDeps(p)
			if err != nil {
				return err
			}
			if daemon.Status(dd).State == "live" {
				daemon.Stop(dd)
			}
			if err := daemon.WriteCLI(dd); err != nil {
				return err
			}
			msg, err := service.Install(d)
			if err == nil {
				fmt.Println(msg)
			}
			return err
		case has(rest, "uninstall"):
			return service.Uninstall(d)
		case has(rest, "restart"):
			return service.Restart(d)
		}
		return errors.New("service install | uninstall | restart")
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
	case "start", "stop":
		d, err := runtime.DaemonDeps(p)
		if err != nil {
			return err
		}
		if sd, err := runtime.ServiceDeps(); err == nil && service.Installed(sd) && cmd == "start" {
			return errors.New("the scheduler runs as a service; use: herdr-triggers service restart")
		}
		run := daemon.Start
		if cmd == "stop" {
			run = daemon.Stop
		}
		msg, err := run(d)
		if err == nil {
			fmt.Println(msg)
		}
		return err
	case "status":
		d, err := runtime.DaemonDeps(p)
		if err != nil {
			return err
		}
		sd, err := runtime.ServiceDeps()
		if err != nil {
			return err
		}
		db, err := store.Open(p.DB)
		if err != nil {
			return err
		}
		defer db.Close()
		cfg, _ := config.Load(p.Config)
		st := status{State: view.Scheduler(db, view.IsScheduler), Mode: "space", Backend: cfg.ResolveBackend(exec.LookPath), Terminal: daemon.Status(d).Terminal}
		if service.Installed(sd) {
			st.Mode = "service"
		}
		if has(rest, "--json") {
			return printJSON(st)
		}
		fmt.Println(st.State, st.Mode, "backend:"+st.Backend, st.Terminal)
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
	case "public":
		cfg, err := config.Load(p.Config)
		if err != nil {
			return err
		}
		c := cfg.Public
		l := view.List(p.Tasks, a.DB, time.Now())
		fmt.Printf("provider %s %s%s\n", c.Provider, c.BaseURL, c.Domain)
		if l.PublicURL != "" {
			fmt.Println("url", l.PublicURL)
		}
		if l.PublicError != "" {
			fmt.Println("error", l.PublicError)
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
		return serve(a, p)
	case "tui":
		return tui.Run(p, a)
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
}

func clearPublic(a *app.App) {
	a.DB.SetMeta(view.PublicURLKey, "")
	a.DB.SetMeta(view.PublicErrorKey, "")
}

type status struct {
	State    string `json:"state"`
	Mode     string `json:"mode"`
	Backend  string `json:"backend"`
	Terminal string `json:"terminal,omitempty"`
}

func expose(a *app.App, p store.Paths) func() {
	clearPublic(a)
	addr := runtime.Addr()
	cfg, err := config.Load(p.Config)
	if err == nil {
		var url string
		var stop func()
		url, stop, err = tunnel.Open(cfg.Public, addr, runtime.Start, 45*time.Second)
		if err == nil {
			a.DB.SetMeta(view.PublicURLKey, url)
			a.Log("webhooks on " + view.HookURL(url, "<id>", "<token>"))
			return stop
		}
	}
	a.Log("public url: " + err.Error())
	a.Notify("Triggers: public url failed", err.Error())
	a.DB.SetMeta(view.PublicURLKey, "http://"+addr)
	a.DB.SetMeta(view.PublicErrorKey, err.Error())
	return func() {}
}

func serve(a *app.App, p store.Paths) error {
	a.Log("scheduler up, tasks: " + a.Tasks)
	a.Reconcile()
	srv := &http.Server{Addr: runtime.Addr(), Handler: webhook.Handler(a, func(f func()) { go f() }), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.Log("webhook server: " + err.Error())
		}
	}()
	stop := expose(a, p)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		<-sig
		stop()
		clearPublic(a)
		view.MarkDown(a.DB)
		os.Exit(0)
	}()
	a.Log("backend: " + a.Dispatch.Backend)
	view.MarkUp(a.DB, os.Getpid())
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
