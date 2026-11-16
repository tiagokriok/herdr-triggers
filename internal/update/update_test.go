package update

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/store"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "0.1.0", true},
		{"v0.10.0", "v0.9.3", true},
		{"v1.0.0", "0.99.99", true},
		{"v0.2.0", "0.2.0", false},
		{"v0.1.9", "0.2.0", false},
		{"v0.2.0", "dev", false},
		{"", "0.1.0", false},
		{"garbage", "0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

func TestCommandFor(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	home := map[string]string{"HOME": "/h"}
	cases := map[string]struct {
		exe  string
		env  map[string]string
		want string
	}{
		"brew mac arm":   {"/opt/homebrew/bin/herdr-triggers", home, BrewCommand},
		"brew caskroom":  {"/opt/homebrew/Caskroom/herdr-triggers/0.2.0/herdr-triggers", home, BrewCommand},
		"brew intel":     {"/usr/local/Caskroom/herdr-triggers/0.2.0/herdr-triggers", home, BrewCommand},
		"brew prefix":    {"/custom/bin/herdr-triggers", map[string]string{"HOMEBREW_PREFIX": "/custom"}, BrewCommand},
		"go default":     {"/h/go/bin/herdr-triggers", home, GoCommand},
		"gopath":         {"/g/bin/herdr-triggers", map[string]string{"GOPATH": "/g"}, GoCommand},
		"gobin":          {"/b/herdr-triggers", map[string]string{"GOBIN": "/b"}, GoCommand},
		"install script": {"/h/.local/bin/herdr-triggers", home, ScriptCommand},
	}
	for name, c := range cases {
		if got := CommandFor(c.exe, env(c.env)); got != c.want {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
}

func TestEnabled(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	if !Enabled(true, "0.1.0", env("")) {
		t.Fatal("default should be on")
	}
	if Enabled(false, "0.1.0", env("")) || Enabled(true, "0.1.0", env("1")) || Enabled(true, "dev", env("")) {
		t.Fatal("config off, env off and dev builds must not check")
	}
}

func TestAvailable(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	if Available(db, "0.1.0", "/h/.local/bin/herdr-triggers", func(string) string { return "" }) != nil {
		t.Fatal("update before any check")
	}
	Record(db, now, "v0.2.0")
	info := Available(db, "0.1.0", "/h/.local/bin/herdr-triggers", func(string) string { return "" })
	if info == nil || info.Latest != "v0.2.0" || info.Command != ScriptCommand {
		t.Fatalf("%+v", info)
	}
	if Available(db, "0.2.0", "/x", func(string) string { return "" }) != nil {
		t.Fatal("update shown when current")
	}
	if Due(db, now.Add(23*time.Hour)) || !Due(db, now.Add(24*time.Hour)) {
		t.Fatal("check must happen once per 24h")
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.3.1","name":"ignored"}`))
	}))
	defer srv.Close()
	if tag, err := Fetch(srv.Client(), srv.URL); err != nil || tag != "v0.3.1" {
		t.Fatal(tag, err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer down.Close()
	if _, err := Fetch(down.Client(), down.URL); err == nil {
		t.Fatal("503 accepted")
	}
}
