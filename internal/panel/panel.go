package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tiagokriok/herdr-triggers/internal/store"
)

const (
	Key        = "ctrl-shift-y"
	Action     = "mod.open:triggers"
	Binding    = `"` + Key + `": "` + Action + `"`
	modSource  = "mods/triggers"
	markerName = ".herdr-triggers.json"
)

var (
	ErrNotManaged = errors.New("the Triggers panel was not installed by herdr-triggers")
	globalOpen    = regexp.MustCompile(`"global"\s*:\s*\{`)
	ourBinding    = regexp.MustCompile(`\s*"` + regexp.QuoteMeta(Key) + `"\s*:\s*"` + regexp.QuoteMeta(Action) + `"\s*,?`)
)

type marker struct {
	Keys bool `json:"keys"`
}

func AppDir(home string) string   { return filepath.Join(home, ".config", "herdr-app") }
func Dir(home string) string      { return filepath.Join(AppDir(home), "mods", "triggers") }
func KeysFile(home string) string { return filepath.Join(AppDir(home), "keys.json") }

func readMarker(home string) (marker, bool) {
	var m marker
	data, err := os.ReadFile(filepath.Join(Dir(home), markerName))
	if err != nil {
		return m, false
	}
	return m, json.Unmarshal(data, &m) == nil
}

func Managed(home string) bool {
	if info, err := os.Lstat(Dir(home)); err != nil || info.Mode()&fs.ModeSymlink != 0 {
		return false
	}
	_, ok := readMarker(home)
	return ok
}

func copyPanel(assets fs.FS, home string, m marker) error {
	dir := Dir(home)
	tmp := dir + ".new"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	err := fs.WalkDir(assets, modSource, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(tmp, filepath.Base(path)), data, 0o644)
	})
	if err == nil {
		err = store.WriteJSON(filepath.Join(tmp, markerName), m)
	}
	if err == nil {
		err = os.RemoveAll(dir)
	}
	if err == nil {
		err = os.Rename(tmp, dir)
	}
	if err != nil {
		os.RemoveAll(tmp)
	}
	return err
}

func Install(assets fs.FS, home string) error {
	dir := Dir(home)
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return err
		}
		return copyPanel(assets, home, marker{})
	case err != nil:
		return err
	case info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%s is a symlink (a dev checkout?); left alone", dir)
	}
	m, ok := readMarker(home)
	if !ok {
		return fmt.Errorf("%s exists and was not installed by herdr-triggers; remove it first", dir)
	}
	return copyPanel(assets, home, m)
}

func Refresh(assets fs.FS, home string) (bool, error) {
	if !Managed(home) {
		return false, nil
	}
	m, _ := readMarker(home)
	return true, copyPanel(assets, home, m)
}

func Bind(home string) (string, error) {
	file := KeysFile(home)
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return "no keys.json yet (open the Herdr app once); add " + Binding + " to its \"global\" section", nil
	}
	if err != nil {
		return "", err
	}
	var keys map[string]map[string]any
	if err := json.Unmarshal(data, &keys); err != nil {
		return "", fmt.Errorf("keys.json: %w", err)
	}
	if action, busy := keys["global"][Key]; busy {
		if action == Action {
			return Key + " already opens the panel", nil
		}
		return fmt.Sprintf("%s is in use (%v); bind the panel yourself: %s", Key, action, Binding), nil
	}
	loc := globalOpen.FindIndex(data)
	if loc == nil {
		return "keys.json has no \"global\" section; add " + Binding + " to it", nil
	}
	rest := strings.TrimLeft(string(data[loc[1]:]), " \t\r\n")
	entry := "\n    " + Binding
	if !strings.HasPrefix(rest, "}") {
		entry += ","
	}
	edited := string(data[:loc[1]]) + entry + string(data[loc[1]:])
	if !json.Valid([]byte(edited)) {
		return "", errors.New("could not edit keys.json safely; add " + Binding + " by hand")
	}
	if err := os.WriteFile(file, []byte(edited), 0o644); err != nil {
		return "", err
	}
	m, _ := readMarker(home)
	m.Keys = true
	return Key + " now opens the panel", store.WriteJSON(filepath.Join(Dir(home), markerName), m)
}

func unbind(home string) error {
	file := KeysFile(home)
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	edited := ourBinding.ReplaceAllString(string(data), "")
	if edited == string(data) {
		return nil
	}
	if !json.Valid([]byte(edited)) {
		return errors.New("could not edit keys.json safely; remove " + Binding + " by hand")
	}
	return os.WriteFile(file, []byte(edited), 0o644)
}

func Uninstall(home string) error {
	if !Managed(home) {
		return ErrNotManaged
	}
	if m, _ := readMarker(home); m.Keys {
		if err := unbind(home); err != nil {
			return err
		}
	}
	return os.RemoveAll(Dir(home))
}

func InstallSkill(assets fs.FS, dir string) (string, error) {
	data, err := fs.ReadFile(assets, "skills/schedule-local/SKILL.md")
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, "schedule-local", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	return target, os.WriteFile(target, data, 0o644)
}
