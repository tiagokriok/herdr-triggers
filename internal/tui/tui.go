package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/tiagokriok/herdr-triggers/internal/app"
	"github.com/tiagokriok/herdr-triggers/internal/daemon"
	"github.com/tiagokriok/herdr-triggers/internal/runtime"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
	"github.com/tiagokriok/herdr-triggers/internal/view"
)

type mode int

const (
	listMode mode = iota
	detailMode
	formMode
	deleteMode
)

const refreshEvery = 2 * time.Second

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	dimStyle   = lipgloss.NewStyle().Faint(true)
	selStyle   = lipgloss.NewStyle().Reverse(true)
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	statusText = map[string]lipgloss.Style{
		"working": lipgloss.NewStyle().Foreground(lipgloss.Color("4")),
		"blocked": lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		"done":    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		"failed":  errStyle, "error": errStyle, "timeout": errStyle, "missed": errStyle,
	}
)

type model struct {
	paths   store.Paths
	app     *app.App
	daemon  daemon.Deps
	listing view.Listing
	sched   daemon.State
	cursor  int
	mode    mode
	form    *huh.Form
	values  *form
	editing string
	message string
	runs    []store.Run
	hooks   []store.Delivery
	width   int
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
	m.cursor = min(m.cursor, max(0, len(m.listing.Tasks)-1))
	if m.mode == detailMode {
		m.loadDetail()
	}
}

func (m *model) selected() (view.Task, bool) {
	if len(m.listing.Tasks) == 0 {
		return view.Task{}, false
	}
	return m.listing.Tasks[m.cursor], true
}

func (m *model) loadDetail() {
	t, ok := m.selected()
	if !ok {
		return
	}
	m.runs, _ = m.app.DB.Runs(t.ID, 10)
	m.hooks, _ = m.app.DB.Deliveries(t.ID, 5)
}

func (m *model) Init() tea.Cmd { return refresh() }

func (m *model) openForm(t task.Task) tea.Cmd {
	v := fromTask(t)
	m.values = &v
	m.form = buildForm(m.values, m.editing != "", m.taken)
	m.mode = formMode
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

func (m *model) save() {
	t, err := m.values.toTask()
	if err == nil {
		err = saveTask(m.paths.Tasks, t)
	}
	if err != nil {
		m.message = errStyle.Render("not saved: " + err.Error())
		return
	}
	m.message = "saved " + t.ID
	if t.Schedule.Type == "webhook" {
		m.message += fmt.Sprintf(" · POST http://%s/hooks/%s with Authorization: Bearer <token> (d to see it)", runtime.Addr(), t.ID)
	}
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

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if w, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = w.Width
	}
	if _, ok := msg.(tickMsg); ok {
		if m.mode != formMode {
			m.reload()
		}
		return m, refresh()
	}
	if m.mode == formMode {
		next, cmd := m.form.Update(msg)
		m.form = next.(*huh.Form)
		switch m.form.State {
		case huh.StateCompleted:
			m.save()
			m.mode = listMode
			m.reload()
		case huh.StateAborted:
			m.message = "cancelled"
			m.mode = listMode
		}
		return m, cmd
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	m.message = ""
	if m.mode == deleteMode {
		if t, ok := m.selected(); ok && key.String() == "y" {
			if err := store.DeleteTask(m.paths.Tasks, t.ID); err != nil {
				m.message = errStyle.Render(err.Error())
			} else {
				m.message = "deleted " + t.ID
			}
		}
		m.mode = listMode
		m.reload()
		return m, nil
	}
	t, has := m.selected()
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = listMode
	case "j", "down":
		if m.cursor < len(m.listing.Tasks)-1 {
			m.cursor++
		}
		if m.mode == detailMode {
			m.loadDetail()
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		if m.mode == detailMode {
			m.loadDetail()
		}
	case "enter", "d":
		if has {
			m.mode = detailMode
			m.loadDetail()
		}
	case "a":
		m.editing = ""
		cwd, _ := os.Getwd()
		return m, m.openForm(task.Task{Schedule: task.Schedule{Type: "cron", Expr: "0 9 * * 1-5"}, Enabled: true, Agent: task.DefaultAgent, Cwd: cwd})
	case "e":
		if full, ok := m.findTask(t.ID); has && ok {
			m.editing = t.ID
			return m, m.openForm(full)
		}
	case " ":
		if has {
			if err := store.SetEnabled(m.paths.Tasks, t.ID, !t.Enabled); err != nil {
				m.message = errStyle.Render(err.Error())
			}
			m.reload()
		}
	case "r":
		if has {
			if err := runInBackground(t.ID); err != nil {
				m.message = errStyle.Render(err.Error())
			} else {
				m.message = "started " + t.ID
			}
		}
	case "x":
		if has {
			m.mode = deleteMode
		}
	case "s":
		msg, err := daemon.Start(m.daemon)
		if err != nil {
			m.message = errStyle.Render(err.Error())
		} else {
			m.message = msg
		}
		m.reload()
	}
	return m, nil
}

func statusOf(s string) string {
	if st, ok := statusText[s]; ok {
		return st.Render(s)
	}
	return s
}

func (m *model) header() string {
	state := statusText["done"].Render("● scheduler live " + m.sched.Terminal)
	if m.sched.State != "live" {
		state = errStyle.Render("○ scheduler stopped") + dimStyle.Render(" (s to start)")
	}
	return titleStyle.Render("Herdr Triggers") + "  " + state + "\n" + dimStyle.Render(m.paths.Tasks) + "\n\n"
}

func (m *model) listView() string {
	var b strings.Builder
	if m.listing.Error != "" {
		b.WriteString(errStyle.Render("tasks.json: "+m.listing.Error) + "\n")
	}
	if len(m.listing.Tasks) == 0 {
		b.WriteString(dimStyle.Render("no tasks yet, press a to add one") + "\n")
	}
	for i, t := range m.listing.Tasks {
		on := "●"
		if !t.Enabled {
			on = "○"
		}
		next := "-"
		if t.Next != nil {
			next = t.Next.Local().Format("01-02 15:04")
		}
		last := "-"
		if t.Last != nil {
			last = fmt.Sprintf("#%d %s", t.Last.N, statusOf(t.Last.Status))
		}
		agent := t.Agent
		if t.Model != "" {
			agent += "/" + t.Model
		}
		line := fmt.Sprintf("%s %-18s %-24s next %-11s %-22s %s", on, t.ID, t.Schedule, next, agent, last)
		if i == m.cursor {
			line = selStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m *model) detailView() string {
	t, ok := m.selected()
	if !ok {
		return ""
	}
	full, _ := m.findTask(t.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", titleStyle.Render(t.Title), dimStyle.Render(t.ID))
	fmt.Fprintf(&b, "%s · %s · %s\n", t.Schedule, full.Agent, filepath.Clean(full.Cwd))
	if full.Model != "" {
		fmt.Fprintf(&b, "model %s\n", full.Model)
	}
	if full.Schedule.Type == "webhook" {
		fmt.Fprintf(&b, "\nPOST http://%s/hooks/%s\nAuthorization: Bearer %s\n", runtime.Addr(), t.ID, full.Schedule.Token)
	}
	fmt.Fprintf(&b, "\n%s\n%s\n", titleStyle.Render("Prompt"), full.Prompt)
	b.WriteString("\n" + titleStyle.Render("Runs") + "\n")
	if len(m.runs) == 0 {
		b.WriteString(dimStyle.Render("none yet") + "\n")
	}
	for _, r := range m.runs {
		line := fmt.Sprintf("#%-4d %s  %-8s %s", r.N, r.FiredAt.Local().Format("01-02 15:04"), statusOf(r.Status), r.Terminal)
		if r.Error != "" {
			line += "  " + errStyle.Render(r.Error)
		}
		b.WriteString(line + "\n")
	}
	if len(m.hooks) > 0 {
		b.WriteString("\n" + titleStyle.Render("Webhook deliveries") + "\n")
		for _, d := range m.hooks {
			fmt.Fprintf(&b, "%s  %-12s %6dB  run #%d\n", d.ReceivedAt.Local().Format("01-02 15:04:05"), d.Outcome, d.Bytes, d.RunN)
		}
	}
	return b.String()
}

func (m *model) View() string {
	if m.mode == formMode {
		return m.header() + m.form.View()
	}
	body := m.listView()
	help := "j/k move · enter details · a add · e edit · space on/off · r run now · x delete · s start scheduler · q quit"
	switch m.mode {
	case detailMode:
		body = m.detailView()
		help = "j/k other task · e edit · r run now · esc back · q quit"
	case deleteMode:
		t, _ := m.selected()
		body += "\n" + errStyle.Render(fmt.Sprintf("delete %s? y/n", t.ID))
	}
	out := m.header() + body + "\n"
	if m.message != "" {
		out += m.message + "\n"
	}
	return out + dimStyle.Render(help) + "\n"
}

func buildForm(v *form, editing bool, taken func(string) bool) *huh.Form {
	agentOptions := huh.NewOptions("claude", "codex", "pi", "opencode", otherAgent)
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
				return fmt.Errorf("keep %q; add a new task for a new id", original)
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
	return huh.NewForm(
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
			huh.NewSelect[string]().Title("Trigger").Options(huh.NewOptions("cron", "once", "webhook")...).Value(&v.Trigger),
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
			huh.NewNote().Title("Webhook").Description(fmt.Sprintf("POST http://%s/hooks/<id>\nA token is generated on save if empty; see it in the task details.", runtime.Addr())),
		).WithHideFunc(func() bool { return v.Trigger != "webhook" }),
		huh.NewGroup(
			huh.NewSelect[string]().Title("Agent").Options(agentOptions...).Value(&v.Agent),
		),
		huh.NewGroup(
			huh.NewInput().Title("Agent command").Description("binary on PATH; the prompt is its last argument").Value(&v.OtherAgent).Validate(required("agent")),
		).WithHideFunc(func() bool { return v.Agent != otherAgent }),
		huh.NewGroup(
			huh.NewInput().Title("Model").Description("optional, passed as the agent's model flag").Value(&v.Model),
			huh.NewInput().Title("Extra agent flags").Description("optional, space separated").Value(&v.AgentArgs),
			huh.NewInput().Title("Timeout (minutes)").Description("optional; the run is killed after it").Value(&v.Timeout),
			huh.NewInput().Title("Catch-up (minutes)").Description("how late a run missed during sleep may still start").Value(&v.CatchUp),
			huh.NewConfirm().Title("Enabled").Value(&v.Enabled),
		),
	)
}
