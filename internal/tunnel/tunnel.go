package tunnel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/store"
)

var Providers = []string{"none", "url", "tailscale", "ngrok", "cloudflared"}

type Config struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl,omitempty"`
	Domain   string `json:"domain,omitempty"`
}

type Starter func(argv []string) (lines <-chan string, stop func(), err error)

var patterns = map[string]*regexp.Regexp{
	"tailscale":   regexp.MustCompile(`(https://[a-zA-Z0-9.-]+\.ts\.net)`),
	"cloudflared": regexp.MustCompile(`(https://[a-z0-9-]+\.trycloudflare\.com)`),
	"ngrok":       regexp.MustCompile(`url=(https://\S+)`),
}

var ready = map[string]*regexp.Regexp{
	"cloudflared": regexp.MustCompile(`Registered tunnel connection`),
}

func (c Config) provider() string {
	if c.Provider == "" {
		return "none"
	}
	return c.Provider
}

func (c Config) Validate() error {
	if !slices.Contains(Providers, c.provider()) {
		return fmt.Errorf("provider must be one of %s", strings.Join(Providers, ", "))
	}
	if c.provider() == "url" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return errors.New("baseUrl must be an http(s) URL")
		}
	}
	return nil
}

func Load(file string) (Config, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{Provider: "none"}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("config.json: %w", err)
	}
	c.Provider = c.provider()
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("config.json: %w", err)
	}
	return c, nil
}

func Save(file string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return store.WriteJSON(file, c)
}

func argv(c Config, addr string) []string {
	switch c.Provider {
	case "tailscale":
		_, port, _ := net.SplitHostPort(addr)
		return []string{"tailscale", "funnel", port}
	case "cloudflared":
		return []string{"cloudflared", "tunnel", "--no-autoupdate", "--protocol", "http2", "--url", "http://" + addr}
	case "ngrok":
		a := []string{"ngrok", "http", addr, "--log", "stdout"}
		if c.Domain != "" {
			a = append(a, "--url", "https://"+c.Domain)
		}
		return a
	}
	return nil
}

func Open(c Config, addr string, start Starter, wait time.Duration) (string, func(), error) {
	switch c.provider() {
	case "none":
		return "http://" + addr, func() {}, nil
	case "url":
		return strings.TrimRight(c.BaseURL, "/"), func() {}, nil
	}
	lines, stop, err := start(argv(c, addr))
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", c.Provider, err)
	}
	var seen []string
	found := ""
	deadline := time.After(wait)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				stop()
				return "", nil, fmt.Errorf("%s exited: %s", c.Provider, strings.Join(seen, " | "))
			}
			if m := patterns[c.Provider].FindStringSubmatch(line); m != nil && found == "" {
				found = m[1]
			}
			if found != "" && (ready[c.Provider] == nil || ready[c.Provider].MatchString(line)) {
				go func() {
					for range lines {
					}
				}()
				return found, stop, nil
			}
			seen = append(seen[max(0, len(seen)-4):], strings.TrimSpace(line))
		case <-deadline:
			stop()
			return "", nil, fmt.Errorf("%s: no public url after %s: %s", c.Provider, wait, strings.Join(seen, " | "))
		}
	}
}
