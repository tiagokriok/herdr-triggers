package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/tiagokriok/herdr-triggers/internal/app"
	"github.com/tiagokriok/herdr-triggers/internal/daemon"
	"github.com/tiagokriok/herdr-triggers/internal/runtime"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
	"github.com/tiagokriok/herdr-triggers/internal/tunnel"
	"github.com/tiagokriok/herdr-triggers/internal/view"
)

type mode int

const (
	listMode mode = iota
	formMode
	publicMode
	deleteMode
)

const (
	refreshEvery = 2 * time.Second
	listHelp     = "↑↓ move · a add · e edit · space on/off · r run · c copy url · t token · x delete · p public url · s start/restart · q quit"
)

type model struct {
	paths   store.Paths
	app     *app.App
	daemon  daemon.Deps
	listing view.Listing
	sched   daemon.State
	config  tunnel.Config
	full    task.Task
	runs    []store.Run
	hooks   []store.Delivery
	cursor  int
	mode    mode
	reveal  bool
	form    *huh.Form
	values  *form
	public  *tunnel.Config
	editing string
	message string
	width   int
	height  int
}

type tickMsg struct{}

func refresh() tea.Cmd { return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} }) }

func Run(p store.Paths, a *app.App) error {
	d, err := runtime.DaemonDeps(p)
	if err != nil {
		return err
	}
	m := &model{paths: p, app: a, daemon: d}
	m.reload()
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m *model) reload() {
	m.listing = view.List(m.paths.Tasks, m.app.DB, time.Now())
	m.sched = daemon.Status(m.daemon)
	m.config, _ = tunnel.Load(m.paths.Config)
	m.cursor = min(m.cursor, max(0, len(m.listing.Tasks)-1))
	m.loadSelected()
}

func (m *model) loadSelected() {
	m.full, m.runs, m.hooks = task.Task{}, nil, nil
	t, ok := m.selected()
	if !ok {
		return
	}
	m.full, _ = m.findTask(t.ID)
	m.runs, _ = m.app.DB.Runs(t.ID, 4)
	m.hooks, _ = m.app.DB.Deliveries(t.ID, 3)
}

func (m *model) selected() (view.Task, bool) {
	if len(m.listing.Tasks) == 0 {
		return view.Task{}, false
	}
	return m.listing.Tasks[m.cursor], true
}

func (m *model) publicURL() string {
	if m.sched.State != "live" {
		return ""
	}
	return m.listing.PublicURL
}

func (m *model) Init() tea.Cmd { return refresh() }

func (m *model) openForm(t task.Task) tea.Cmd {
	v := fromTask(t)
	m.values = &v
	base := m.publicURL()
	if base == "" {
		base = "http://" + runtime.Addr()
	}
	m.form = buildForm(m.values, m.editing != "", m.taken, base, quickTunnel(m.config)).WithTheme(huh.ThemeCharm())
	m.mode = formMode
	return m.form.Init()
}

func (m *model) openPublic() tea.Cmd {
	c := m.config
	m.public = &c
	m.form = buildPublicForm(m.public).WithTheme(huh.ThemeCharm())
	m.mode = publicMode
	return m.form.Init()
}

func (m *model) taken(id string) bool {
	for _, t := range m.listing.Tasks {
		if t.ID == id {
			return true
		}
	}
	return false
}

func (m *model) findTask(id string) (task.Task, bool) {
	f, err := store.LoadTasks(m.paths.Tasks)
	if err != nil {
		return task.Task{}, false
	}
	for _, t := range f.Tasks {
		if t.ID == id {
			return t, true
		}
	}
	return task.Task{}, false
}

func (m *model) fail(err error) { m.message = red.Render(err.Error()) }

func (m *model) save() {
	t, err := m.values.toTask()
	if err == nil {
		err = saveTask(m.paths.Tasks, t)
	}
	if err != nil {
		m.fail(fmt.Errorf("not saved: %w", err))
		return
	}
	m.message = green.Render("saved " + t.ID)
	if t.Schedule.Type == "webhook" {
		m.message += dim.Render(" · c copies its url")
	}
	for i, x := range view.List(m.paths.Tasks, m.app.DB, time.Now()).Tasks {
		if x.ID == t.ID {
			m.cursor = i
		}
	}
}

func (m *model) savePublic() {
	if err := tunnel.Save(m.paths.Config, *m.public); err != nil {
		m.fail(fmt.Errorf("not saved: %w", err))
		return
	}
	m.message = green.Render("public url: "+m.public.Provider) + dim.Render(" · s restarts the scheduler to apply")
}

func saveTask(file string, t task.Task) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return store.SaveTask(file, raw)
}

func runInBackground(id string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return exec.Command(exe, "run-now", id, "--bg").Run()
}

func copyText(s string) error {
	for _, argv := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"pbcopy"}} {
		if _, err := exec.LookPath(argv[0]); err != nil {
			continue
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin = strings.NewReader(s)
		return cmd.Run()
	}
	return errors.New("no clipboard tool (wl-copy, xclip, pbcopy); press t to show the token")
}

func (m *model) restart() {
	if m.sched.State == "live" {
		if _, err := daemon.Stop(m.daemon); err != nil {
			m.fail(err)
			return
		}
	}
	msg, err := daemon.Start(m.daemon)
	if err != nil {
		m.fail(err)
		return
	}
	m.message = green.Render(msg)
}

func (m *model) updateForm(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
		m.message = dim.Render("cancelled")
		m.mode = listMode
		return nil
	}
	next, cmd := m.form.Update(msg)
	m.form = next.(*huh.Form)
	switch m.form.State {
	case huh.StateCompleted:
		if m.mode == formMode {
			m.save()
		} else {
			m.savePublic()
		}
		m.mode = listMode
		m.reload()
	case huh.StateAborted:
		m.message = dim.Render("cancelled")
		m.mode = listMode
	}
	return cmd
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if w, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = w.Width, w.Height
	}
	if _, ok := msg.(tickMsg); ok {
		if m.mode == listMode {
			m.reload()
		}
		return m, refresh()
	}
	if m.mode == formMode || m.mode == publicMode {
		return m, m.updateForm(msg)
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	m.message = ""
	t, has := m.selected()
	if m.mode == deleteMode {
		if has && key.String() == "y" {
			if err := store.DeleteTask(m.paths.Tasks, t.ID); err != nil {
				m.fail(err)
			} else {
				m.message = "deleted " + t.ID
			}
		}
		m.mode = listMode
		m.reload()
		return m, nil
	}
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor = min(m.cursor+1, max(0, len(m.listing.Tasks)-1))
		m.reveal = false
		m.loadSelected()
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
		m.reveal = false
		m.loadSelected()
	case "a":
		m.editing = ""
		cwd, _ := os.Getwd()
		return m, m.openForm(task.Task{Schedule: task.Schedule{Type: "cron", Expr: "0 9 * * 1-5"}, Enabled: true, Agent: task.DefaultAgent, Cwd: cwd})
	case "e", "enter":
		if has && m.full.ID == t.ID {
			m.editing = t.ID
			return m, m.openForm(m.full)
		}
	case " ":
		if has {
			if err := store.SetEnabled(m.paths.Tasks, t.ID, !t.Enabled); err != nil {
				m.fail(err)
			}
			m.reload()
		}
	case "r":
		if has {
			if err := runInBackground(t.ID); err != nil {
				m.fail(err)
			} else {
				m.message = "started " + t.ID
			}
		}
	case "t":
		m.reveal = !m.reveal
	case "c":
		if has && m.full.Schedule.Type == "webhook" {
			url := view.HookURL(m.publicURL(), m.full.ID, m.full.Schedule.Token)
			if m.publicURL() == "" {
				m.fail(errors.New("scheduler off: start it with s to get the url"))
			} else if err := copyText(url); err != nil {
				m.fail(err)
			} else {
				m.message = green.Render("copied webhook url")
			}
		}
	case "x":
		if has {
			m.mode = deleteMode
			m.message = red.Render(fmt.Sprintf("delete %s? y/n", t.ID))
		}
	case "p":
		return m, m.openPublic()
	case "s":
		m.restart()
		m.reload()
	}
	return m, nil
}

func (m *model) screen() screenData {
	return screenData{
		now: time.Now(), width: m.width, height: m.height, cursor: m.cursor,
		sched: m.sched, public: m.publicURL(), config: m.config, listing: m.listing,
		selected: m.full, runs: m.runs, hooks: m.hooks, reveal: m.reveal,
		publicErr: publicErr(m), message: m.message, help: listHelp,
	}
}

func publicErr(m *model) string {
	if m.sched.State != "live" {
		return ""
	}
	return m.listing.PublicError
}

func (m *model) View() string {
	if m.mode == formMode || m.mode == publicMode {
		s := m.screen()
		width := s.width
		if width <= 0 {
			width = 100
		}
		title := "new trigger"
		switch {
		case m.mode == publicMode:
			title = "public url"
		case m.editing != "":
			title = "edit " + m.editing
		}
		return strings.Join(header(s), "\n") + "\n" + dim.Render(strings.Repeat("─", width)) + "\n" + bold.Render(title) + "\n\n" + m.form.View()
	}
	return render(m.screen())
}

func buildPublicForm(c *tunnel.Config) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().Title("Expose webhooks with").
				Options(
					huh.NewOption("none: localhost only", "none"),
					huh.NewOption("tailscale funnel: https://<machine>.<tailnet>.ts.net", "tailscale"),
					huh.NewOption("ngrok: random url, or your reserved domain", "ngrok"),
					huh.NewOption("cloudflared quick tunnel: random trycloudflare.com url", "cloudflared"),
					huh.NewOption("url: a tunnel or proxy you run yourself", "url"),
				).Value(&c.Provider),
		),
		huh.NewGroup(
			huh.NewInput().Title("Base URL").Description("public URL that reaches http://"+runtime.Addr()).Value(&c.BaseURL).Validate(func(s string) error {
				return tunnel.Config{Provider: "url", BaseURL: s}.Validate()
			}),
		).WithHideFunc(func() bool { return c.Provider != "url" }),
		huh.NewGroup(
			huh.NewInput().Title("Domain").Description("optional reserved ngrok domain, e.g. hooks.ngrok.app; empty = random url").Value(&c.Domain),
		).WithHideFunc(func() bool { return c.Provider != "ngrok" }),
	)
}

func buildForm(v *form, editing bool, taken func(string) bool, base string, quick bool) *huh.Form {
	idInput := huh.NewInput().Title("ID").Description("lowercase, digits and dashes").Value(&v.ID).Validate(func(s string) error {
		probe := task.Task{ID: s, Title: "x", Prompt: "x", Cwd: "/", Schedule: task.Schedule{Type: "webhook", Token: strings.Repeat("x", task.MinTokenLength)}}
		if err := probe.Validate(); err != nil {
			return err
		}
		if !editing && taken(s) {
			return fmt.Errorf("id %q already exists", s)
		}
		return nil
	})
	if editing {
		original := v.ID
		idInput = idInput.Description("the id cannot change while editing").Validate(func(s string) error {
			if s != original {
				return fmt.Errorf("keep %q; add a new trigger for a new id", original)
			}
			return nil
		})
	}
	required := func(name string) func(string) error {
		return func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("%s is required", name)
			}
			return nil
		}
	}
	hookNote := func() string {
		id := v.ID
		if id == "" {
			id = "<id>"
		}
		note := "POST " + view.HookURL(base, id, "<token>") + "\nThe token is generated on save; c copies the full url from the list."
		if quick {
			note += "\nThis provider gives a new url each time the scheduler restarts."
		}
		return note
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().Title("Trigger").Options(
				huh.NewOption("cron: on a schedule", "cron"),
				huh.NewOption("once: at a date and time", "once"),
				huh.NewOption("webhook: when a URL is called", "webhook"),
			).Value(&v.Trigger),
		),
		huh.NewGroup(
			huh.NewInput().Title("Cron").Value(&v.Cron).
				DescriptionFunc(func() string {
					return "5 fields, local time · " + preview(task.Schedule{Type: "cron", Expr: v.Cron}, time.Now())
				}, &v.Cron).
				Validate(task.ValidateCron),
		).WithHideFunc(func() bool { return v.Trigger != "cron" }),
		huh.NewGroup(
			huh.NewInput().Title("Run at").Description("local time, "+localLayout).Value(&v.RunAt).Validate(func(s string) error {
				_, err := time.ParseInLocation(localLayout, strings.TrimSpace(s), time.Local)
				return err
			}),
		).WithHideFunc(func() bool { return v.Trigger != "once" }),
		huh.NewGroup(
			idInput,
			huh.NewInput().Title("Title").Value(&v.Title).Validate(required("title")),
			huh.NewText().Title("Prompt").Description("what the agent should do").Value(&v.Prompt).Validate(required("prompt")),
			huh.NewInput().Title("Folder").Description("absolute path the agent runs in").Value(&v.Cwd).Validate(func(s string) error {
				if !filepath.IsAbs(strings.TrimSpace(s)) {
					return fmt.Errorf("must be an absolute path")
				}
				return nil
			}),
		),
		huh.NewGroup(
			huh.NewNote().Title("Webhook URL").DescriptionFunc(hookNote, &v.ID),
		).WithHideFunc(func() bool { return v.Trigger != "webhook" }),
		huh.NewGroup(
			huh.NewSelect[string]().Title("Agent").Options(huh.NewOptions("claude", "codex", "pi", "opencode", otherAgent)...).Value(&v.Agent),
			huh.NewInput().Title("Model").Description("optional, passed as the agent's model flag").Value(&v.Model),
		),
		huh.NewGroup(
			huh.NewInput().Title("Agent command").Description("binary on PATH; the prompt is its last argument").Value(&v.OtherAgent).Validate(required("agent")),
		).WithHideFunc(func() bool { return v.Agent != otherAgent }),
		huh.NewGroup(
			huh.NewInput().Title("Extra agent flags").Description("optional, space separated").Value(&v.AgentArgs),
			huh.NewInput().Title("Timeout (minutes)").Description("optional; the run is killed after it").Value(&v.Timeout),
			huh.NewInput().Title("Catch-up (minutes)").Description("how late a run missed during sleep may still start").Value(&v.CatchUp),
			huh.NewConfirm().Title("Enabled").Value(&v.Enabled),
		),
	)
}
