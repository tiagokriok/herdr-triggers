package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/store"
)

const (
	LatestURL     = "https://api.github.com/repos/tiagokriok/herdr-triggers/releases/latest"
	Every         = 24 * time.Hour
	restart       = " && herdr-triggers service restart"
	BrewCommand   = "brew upgrade --cask herdr-triggers" + restart
	GoCommand     = "go install github.com/tiagokriok/herdr-triggers/cmd/herdr-triggers@latest" + restart
	ScriptCommand = "curl -fsSL https://raw.githubusercontent.com/tiagokriok/herdr-triggers/main/install.sh | sh" + restart
	checkedKey    = "update_checked_at"
	latestKey     = "update_latest"
	DisableEnv    = "TRIGGERS_NO_UPDATE_CHECK"
)

type Info struct {
	Latest  string `json:"latest"`
	Command string `json:"command"`
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		p, _, _ = strings.Cut(p, "-")
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func Newer(latest, current string) bool {
	l, ok := parse(latest)
	c, ok2 := parse(current)
	if !ok || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func Enabled(config bool, version string, getenv func(string) string) bool {
	_, release := parse(version)
	return config && release && getenv(DisableEnv) == ""
}

func under(path, dir string) bool {
	return dir != "" && strings.HasPrefix(path, filepath.Clean(dir)+string(filepath.Separator))
}

func CommandFor(exe string, getenv func(string) string) string {
	switch {
	case strings.Contains(exe, "/Caskroom/"), under(exe, "/opt/homebrew"), under(exe, getenv("HOMEBREW_PREFIX")):
		return BrewCommand
	case under(exe, getenv("GOBIN")), under(exe, filepath.Join(getenv("GOPATH"), "bin")) && getenv("GOPATH") != "",
		getenv("HOME") != "" && under(exe, filepath.Join(getenv("HOME"), "go", "bin")):
		return GoCommand
	}
	return ScriptCommand
}

func Due(db *store.DB, now time.Time) bool {
	last, err := time.Parse(time.RFC3339, db.Meta(checkedKey))
	return err != nil || now.Sub(last) >= Every
}

func Record(db *store.DB, now time.Time, latest string) {
	db.SetMeta(checkedKey, now.UTC().Format(time.RFC3339))
	if latest != "" {
		db.SetMeta(latestKey, latest)
	}
}

func Available(db *store.DB, current, exe string, getenv func(string) string) *Info {
	latest := db.Meta(latestKey)
	if !Newer(latest, current) {
		return nil
	}
	return &Info{Latest: latest, Command: CommandFor(exe, getenv)}
}

func Fetch(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest release: %s", res.Status)
	}
	var body struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Tag, nil
}
