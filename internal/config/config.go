package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"slices"

	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/tunnel"
)

var (
	Backends       = []string{"auto", "app", "cli"}
	sessionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type Config struct {
	Backend string        `json:"backend,omitempty"`
	Session string        `json:"session,omitempty"`
	Public  tunnel.Config `json:"public"`
}

func (c *Config) defaults() {
	if c.Backend == "" {
		c.Backend = "auto"
	}
	if c.Public.Provider == "" {
		c.Public.Provider = "none"
	}
}

func (c Config) Validate() error {
	c.defaults()
	if !slices.Contains(Backends, c.Backend) {
		return fmt.Errorf("backend must be one of auto, app, cli")
	}
	if c.Session != "" && !sessionPattern.MatchString(c.Session) {
		return errors.New("session: letters, digits, - and _ only")
	}
	if err := c.Public.Validate(); err != nil {
		return fmt.Errorf("public: %w", err)
	}
	return nil
}

func (c Config) ResolveBackend(lookPath func(string) (string, error)) string {
	if c.Backend == "app" || c.Backend == "cli" {
		return c.Backend
	}
	if _, err := lookPath("herdr-app"); err == nil {
		return "app"
	}
	return "cli"
}

func Load(file string) (Config, error) {
	var c Config
	data, err := os.ReadFile(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Config{}, err
	default:
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			return Config{}, fmt.Errorf("config.json: %w", err)
		}
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("config.json: %w", err)
	}
	c.defaults()
	return c, nil
}

func Save(file string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	c.defaults()
	return store.WriteJSON(file, c)
}
