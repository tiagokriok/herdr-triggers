package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tiagokriok/herdr-triggers/internal/daemon"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
	"github.com/tiagokriok/herdr-triggers/internal/tunnel"
	"github.com/tiagokriok/herdr-triggers/internal/view"
)

type screenData struct {
	now           time.Time
	width, height int
	cursor        int
	sched         daemon.State
	backend       string
	public        string
	publicErr     string
	config        tunnel.Config
	listing       view.Listing
	selected      task.Task
	runs          []store.Run
	hooks         []store.Delivery
	reveal        bool
	message       string
	help          string
}

var (
	accent  = lipgloss.Color("6")
	bold    = lipgloss.NewStyle().Bold(true)
	brand   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	dim     = lipgloss.NewStyle().Faint(true)
	red     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	green   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	blue    = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	label   = lipgloss.NewStyle().Faint(true).Width(9)
	cursorS = lipgloss.NewStyle().Bold(true).Foreground(accent)
	panel   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)
	states  = map[string]struct {
		icon  string
		style lipgloss.Style
	}{
		"done": {"✓", green}, "working": {"●", blue}, "starting": {"◌", blue}, "blocked": {"◐", yellow},
		"failed": {"✗", red}, "error": {"✗", red}, "timeout": {"✗", red}, "missed": {"!", red}, "exit": {"■", dim},
	}
)

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func relTime(now, t time.Time) string {
	now, t = now.In(time.Local), t.In(time.Local)
	d := t.Sub(now)
	if d >= 0 {
		switch {
		case d < time.Minute:
			return "in <1m"
		case d < time.Hour:
			return fmt.Sprintf("in %dm", int(d.Minutes()))
		case sameDay(now, t):
			return fmt.Sprintf("in %dh", int(d.Hours()))
		case sameDay(now.AddDate(0, 0, 1), t):
			return "tomorrow " + t.Format("15:04")
		}
		return t.Format("Mon 01-02 15:04")
	}
	d = -d
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case sameDay(now, t):
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case sameDay(now.AddDate(0, 0, -1), t):
		return "yesterday " + t.Format("15:04")
	}
	return t.Format("01-02 15:04")
}

func mask(url string) string {
	i := strings.LastIndex(url, "/")
	token := url[i+1:]
	return url[:i+1] + token[:min(4, len(token))] + "••••••"
}

func quickTunnel(c tunnel.Config) bool {
	return c.Provider == "cloudflared" || (c.Provider == "ngrok" && c.Domain == "")
}

func runCell(n int, status string) string {
	s, ok := states[status]
	if !ok {
		return fmt.Sprintf("#%d %s", n, status)
	}
	return s.style.Render(fmt.Sprintf("%s #%d %s", s.icon, n, status))
}

func statusCell(t view.Task) string {
	switch {
	case !t.Enabled:
		return dim.Render("○ off")
	case t.Last == nil:
		return dim.Render("– never")
	}
	return runCell(t.Last.N, t.Last.Status)
}

func whenCell(s screenData, t view.Task) string {
	switch {
	case t.Enabled && t.Next != nil:
		return relTime(s.now, *t.Next)
	case t.Last != nil:
		return dim.Render(relTime(s.now, t.Last.FiredAt))
	}
	return dim.Render("—")
}

func cell(s string, w int) string {
	return lipgloss.NewStyle().Width(w).Render(ansi.Truncate(s, w, "…"))
}

func fit(s string, w int) string { return ansi.Truncate(s, w, "…") }

type column struct {
	title string
	width int
	value func(view.Task) string
}

func columns(s screenData) []column {
	agent := func(t view.Task) string {
		if t.Model == "" {
			return t.Agent
		}
		return t.Agent + "·" + t.Model
	}
	cols := []column{
		{"ID", 0, func(t view.Task) string { return t.ID }},
		{"TRIGGER", 24, func(t view.Task) string { return t.Schedule }},
		{"AGENT", 16, agent},
		{"NEXT / LAST", 16, func(t view.Task) string { return whenCell(s, t) }},
		{"STATUS", 14, statusCell},
	}
	if s.width < 90 {
		cols = []column{cols[0], cols[1], cols[4]}
	}
	used := 2
	for _, c := range cols[1:] {
		used += c.width + 1
	}
	cols[0].width = min(32, max(10, s.width-used))
	return cols
}

func row(cols []column, cells []string) string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = cell(cells[i], c.width)
	}
	return strings.Join(out, " ")
}

func header(s screenData) []string {
	state := green.Render("● scheduler on")
	if s.sched.State != "live" {
		state = red.Render("○ scheduler off") + dim.Render(" · s to start")
	}
	if s.backend != "" {
		state += dim.Render(" · runs in " + s.backend)
	}
	if s.public != "" {
		state += dim.Render(" · ") + s.public
	}
	lines := []string{fit(brand.Render("herdr-triggers")+"  "+state, s.width)}
	if s.publicErr != "" {
		lines = append(lines, fit(red.Render("public url failed: "+s.publicErr), s.width))
	}
	return lines
}

func hookURL(s screenData) string {
	base := s.public
	if base == "" {
		base = "http://<scheduler off>"
	}
	url := view.HookURL(base, s.selected.ID, s.selected.Schedule.Token)
	if !s.reveal {
		url = mask(url)
	}
	return url
}

func details(s screenData) []string {
	t := s.selected
	line := func(name, value string) string { return label.Render(name) + value }
	prompt, _, _ := strings.Cut(strings.TrimSpace(t.Prompt), "\n")
	where := t.Cwd
	if t.TimeoutMinutes > 0 {
		where += dim.Render(fmt.Sprintf("  timeout %dm", t.TimeoutMinutes))
	}
	where += dim.Render(fmt.Sprintf("  catch-up %dm", int(t.CatchUp().Minutes())))
	out := []string{bold.Render(t.Title) + "  " + dim.Render(t.ID), line("prompt", prompt), line("cwd", where)}
	if t.Schedule.Type == "webhook" {
		url := hookURL(s)
		if quickTunnel(s.config) {
			url += yellow.Render("  changes on restart")
		}
		out = append(out, line("url", url))
	}
	var runs []string
	for _, r := range s.runs[:min(4, len(s.runs))] {
		runs = append(runs, runCell(r.N, r.Status)+" "+dim.Render(relTime(s.now, r.FiredAt)))
	}
	if len(runs) == 0 {
		runs = []string{dim.Render("none yet")}
	}
	out = append(out, line("runs", strings.Join(runs, dim.Render(" · "))))
	if t.Schedule.Type == "webhook" && len(s.hooks) > 0 {
		var hooks []string
		for _, d := range s.hooks[:min(3, len(s.hooks))] {
			hooks = append(hooks, fmt.Sprintf("%s %dB %s", d.Outcome, d.Bytes, dim.Render(relTime(s.now, d.ReceivedAt))))
		}
		out = append(out, line("hooks", strings.Join(hooks, dim.Render(" · "))))
	}
	return out
}

func render(s screenData) string {
	if s.width <= 0 {
		s.width = 100
	}
	if s.height <= 0 {
		s.height = 40
	}
	rule := dim.Render(strings.Repeat("─", s.width))
	top := header(s)
	var box string
	if len(s.listing.Tasks) > 0 && s.selected.ID != "" {
		inner := s.width - 4
		lines := details(s)
		for i := range lines {
			if strings.HasPrefix(ansi.Strip(lines[i]), "url ") {
				lines[i] = ansi.Hardwrap(lines[i], inner, false)
			} else {
				lines[i] = fit(lines[i], inner)
			}
		}
		box = panel.Width(s.width - 2).Render(strings.Join(lines, "\n"))
	}
	foot := []string{}
	if s.message != "" {
		foot = append(foot, fit(s.message, s.width))
	}
	foot = append(foot, fit(dim.Render(s.help), s.width))

	cols := columns(s)
	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = c.title
	}
	body := []string{"  " + dim.Render(row(cols, titles))}
	if s.listing.Error != "" {
		body = append(body, fit(red.Render("tasks.json: "+s.listing.Error), s.width))
	}
	if len(s.listing.Tasks) == 0 {
		body = append(body, dim.Render("  no triggers yet, press a to add one"))
	}
	fixed := len(top) + 2 + len(body) + len(foot)
	if box != "" {
		fixed += lipgloss.Height(box)
	}
	rows := max(3, s.height-fixed)
	off := max(0, s.cursor-rows+1)
	for i, t := range s.listing.Tasks[off:min(len(s.listing.Tasks), off+rows)] {
		cells := make([]string, len(cols))
		for j, c := range cols {
			cells[j] = c.value(t)
		}
		r := row(cols, cells)
		if off+i == s.cursor {
			body = append(body, cursorS.Render("▸ ")+bold.Render(r))
		} else if !t.Enabled {
			body = append(body, "  "+dim.Render(r))
		} else {
			body = append(body, "  "+r)
		}
	}
	out := append(top, rule)
	out = append(out, body...)
	out = append(out, rule)
	if box != "" {
		out = append(out, box)
	}
	out = append(out, foot...)
	return strings.Join(out, "\n") + "\n"
}
