package store

import (
	"errors"
	"path/filepath"
)

const appDir = "herdr-triggers"

type Paths struct {
	Tasks     string
	CLI       string
	Config    string
	DB        string
	Scheduler string
}

func ResolvePaths(getenv func(string) string) (Paths, error) {
	home := getenv("HOME")
	config, state := getenv("XDG_CONFIG_HOME"), getenv("XDG_STATE_HOME")
	if home == "" && (config == "" || state == "") {
		return Paths{}, errors.New("HOME is not set")
	}
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	return Paths{
		Tasks:     filepath.Join(config, appDir, "tasks.json"),
		CLI:       filepath.Join(config, appDir, "cli.json"),
		Config:    filepath.Join(config, appDir, "config.json"),
		DB:        filepath.Join(state, appDir, "state.db"),
		Scheduler: filepath.Join(state, appDir, "scheduler.json"),
	}, nil
}
