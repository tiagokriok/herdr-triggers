package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/adhocore/gronx"
)

const (
	DefaultAgent          = "claude"
	DefaultCatchUpMinutes = 120
	MinTokenLength        = 16
)

type Schedule struct {
	Type  string `json:"type"`
	Expr  string `json:"expr,omitempty"`
	RunAt string `json:"runAt,omitempty"`
	Token string `json:"token,omitempty"`
}

type Task struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Prompt          string   `json:"prompt"`
	Cwd             string   `json:"cwd"`
	Schedule        Schedule `json:"schedule"`
	Enabled         bool     `json:"enabled"`
	Agent           string   `json:"agent,omitempty"`
	Model           string   `json:"model,omitempty"`
	AgentArgs       []string `json:"agentArgs,omitempty"`
	TimeoutMinutes  int      `json:"timeoutMinutes,omitempty"`
	CatchUpMinutes  *int     `json:"catchUpMinutes,omitempty"`
	ClaudeConfigDir string   `json:"claudeConfigDir,omitempty"`
}

type File struct {
	Tasks []Task `json:"tasks"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func ValidateCron(expr string) error {
	if len(strings.Fields(expr)) != 5 {
		return errors.New("cron must have exactly 5 fields (no seconds)")
	}
	if !gronx.IsValid(expr) {
		return fmt.Errorf("invalid cron expression %q", expr)
	}
	return nil
}

func (t Task) CatchUp() time.Duration {
	if t.CatchUpMinutes == nil {
		return DefaultCatchUpMinutes * time.Minute
	}
	return time.Duration(*t.CatchUpMinutes) * time.Minute
}

func (s Schedule) validate() error {
	switch s.Type {
	case "cron":
		return ValidateCron(s.Expr)
	case "once":
		if _, err := time.Parse(time.RFC3339, s.RunAt); err != nil {
			return fmt.Errorf("runAt must be ISO 8601 with offset: %q", s.RunAt)
		}
		return nil
	case "webhook":
		if len(s.Token) < MinTokenLength {
			return fmt.Errorf("webhook token must have at least %d characters", MinTokenLength)
		}
		return nil
	}
	return fmt.Errorf("unknown schedule type %q (cron, once, webhook)", s.Type)
}

func (t *Task) validate() error {
	switch {
	case !idPattern.MatchString(t.ID):
		return fmt.Errorf("id must match %s", idPattern)
	case strings.TrimSpace(t.Title) == "":
		return errors.New("title is required")
	case strings.TrimSpace(t.Prompt) == "":
		return errors.New("prompt is required")
	case !filepath.IsAbs(t.Cwd):
		return fmt.Errorf("cwd must be an absolute path: %q", t.Cwd)
	case t.TimeoutMinutes < 0:
		return errors.New("timeoutMinutes must be positive")
	case t.CatchUpMinutes != nil && *t.CatchUpMinutes < 0:
		return errors.New("catchUpMinutes must be 0 or more")
	}
	if err := t.Schedule.validate(); err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	if t.Agent == "" {
		t.Agent = DefaultAgent
	}
	if t.CatchUpMinutes == nil {
		n := DefaultCatchUpMinutes
		t.CatchUpMinutes = &n
	}
	return nil
}

func Parse(data []byte) (File, error) {
	var f File
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("invalid json: %w", err)
	}
	seen := map[string]bool{}
	for i := range f.Tasks {
		if err := f.Tasks[i].validate(); err != nil {
			return File{}, fmt.Errorf("tasks[%d] (%s): %w", i, f.Tasks[i].ID, err)
		}
		if seen[f.Tasks[i].ID] {
			return File{}, fmt.Errorf("duplicate task id %q", f.Tasks[i].ID)
		}
		seen[f.Tasks[i].ID] = true
	}
	return f, nil
}

func (t Task) Validate() error {
	return t.validate()
}
