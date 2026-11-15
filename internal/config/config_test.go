package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiagokriok/herdr-triggers/internal/tunnel"
)

func TestLoadSave(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	if c, err := Load(file); err != nil || c.Backend != "auto" || c.Public.Provider != "none" {
		t.Fatal(c, err)
	}
	if err := Save(file, Config{Public: tunnel.Config{Provider: "url"}}); err == nil || !strings.Contains(err.Error(), "baseUrl") {
		t.Fatal(err)
	}
	want := Config{Backend: "cli", Session: "hooks", Public: tunnel.Config{Provider: "ngrok", Domain: "d.ngrok.app"}}
	if err := Save(file, want); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(file); err != nil || c != want {
		t.Fatal(c, err)
	}
	for _, bad := range []string{
		`{"public":{"provider":"frp"}}`,
		`{"backend":"tmux"}`,
		`{"x":1}`,
		`{"public":{"provider":"url","baseUrl":"ftp://x"}}`,
		`{"session":"bad name!"}`,
	} {
		os.WriteFile(file, []byte(bad), 0o644)
		if _, err := Load(file); err == nil {
			t.Error(bad)
		}
	}
}

func TestResolveBackend(t *testing.T) {
	has := func(names ...string) func(string) (string, error) {
		return func(n string) (string, error) {
			for _, x := range names {
				if x == n {
					return "/bin/" + n, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	cases := []struct {
		backend string
		path    []string
		want    string
	}{
		{"auto", []string{"herdr-app", "herdr"}, "app"},
		{"auto", []string{"herdr"}, "cli"},
		{"auto", nil, "cli"},
		{"cli", []string{"herdr-app"}, "cli"},
		{"app", nil, "app"},
	}
	for _, c := range cases {
		if got := (Config{Backend: c.backend}).ResolveBackend(has(c.path...)); got != c.want {
			t.Errorf("%+v: %s", c, got)
		}
	}
}
