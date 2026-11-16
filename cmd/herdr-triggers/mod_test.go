package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tiagokriok/herdr-triggers/internal/dispatch"
)

type fakeExec struct{ calls [][]string }

func (f *fakeExec) install(t *testing.T) {
	prev := execFn
	execFn = func(argv []string) dispatch.Result {
		f.calls = append(f.calls, argv)
		return dispatch.Result{}
	}
	t.Cleanup(func() { execFn = prev })
}

func (f *fakeExec) ran(prefix ...string) bool {
	for _, c := range f.calls {
		if len(c) >= len(prefix) && slices.Equal(c[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

func (s sandbox) modDir() string {
	return filepath.Join(s.home, ".config", "herdr-app", "mods", "triggers")
}
func (s sandbox) keysFile() string { return filepath.Join(s.home, ".config", "herdr-app", "keys.json") }

func readKeys(t *testing.T, s sandbox) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(s.keysFile())
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]map[string]any
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatalf("keys.json invalid: %v\n%s", err, data)
	}
	return keys
}

func TestModInstall(t *testing.T) {
	s := newSandbox(t)
	ex := &fakeExec{}
	ex.install(t)
	out := must(t, "mod", "install")
	for _, f := range []string{"main.js", "mod.json", "icon.svg"} {
		if _, err := os.Stat(filepath.Join(s.modDir(), f)); err != nil {
			t.Fatalf("%s not installed: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.modDir(), "preview.png")); err == nil {
		t.Fatal("preview.png shipped in the binary")
	}
	if !ex.ran("herdr-app", "check", s.modDir()) {
		t.Fatalf("herdr-app check not run: %v", ex.calls)
	}
	cli, err := os.ReadFile(filepath.Join(s.config, "herdr-triggers", "cli.json"))
	if err != nil || !strings.Contains(string(cli), "argv") {
		t.Fatalf("cli.json: %s %v", cli, err)
	}
	if !strings.Contains(out, `"ctrl-shift-y": "mod.open:triggers"`) {
		t.Fatalf("binding hint missing: %q", out)
	}
	if _, err := os.Stat(s.keysFile()); err == nil {
		t.Fatal("keys.json written without --keys")
	}
}

func TestModInstallKeys(t *testing.T) {
	s := newSandbox(t)
	(&fakeExec{}).install(t)
	os.MkdirAll(filepath.Dir(s.keysFile()), 0o755)
	os.WriteFile(s.keysFile(), []byte("{\n  \"global\": {\n    \"ctrl-shift-t\": \"tab.new\"\n  },\n  \"terminal\": {}\n}\n"), 0o644)
	must(t, "mod", "install", "--keys")
	keys := readKeys(t, s)
	if keys["global"]["ctrl-shift-y"] != "mod.open:triggers" || keys["global"]["ctrl-shift-t"] != "tab.new" {
		t.Fatalf("%v", keys)
	}
	must(t, "mod", "uninstall")
	keys = readKeys(t, s)
	if _, ok := keys["global"]["ctrl-shift-y"]; ok || keys["global"]["ctrl-shift-t"] != "tab.new" {
		t.Fatalf("after uninstall: %v", keys)
	}
	if _, err := os.Stat(s.modDir()); err == nil {
		t.Fatal("mod dir left after uninstall")
	}
}

func TestModUninstallBindingLast(t *testing.T) {
	s := newSandbox(t)
	(&fakeExec{}).install(t)
	os.MkdirAll(filepath.Dir(s.keysFile()), 0o755)
	os.WriteFile(s.keysFile(), []byte(`{"global":{"ctrl-shift-t":"tab.new"}}`), 0o644)
	must(t, "mod", "install", "--keys")
	os.WriteFile(s.keysFile(), []byte("{\n  \"global\": {\n    \"ctrl-shift-t\": \"tab.new\",\n    \"ctrl-shift-y\": \"mod.open:triggers\"\n  }\n}\n"), 0o644)
	must(t, "mod", "uninstall")
	keys := readKeys(t, s)
	if _, ok := keys["global"]["ctrl-shift-y"]; ok || keys["global"]["ctrl-shift-t"] != "tab.new" {
		t.Fatalf("%v", keys)
	}
}

func TestModInstallKeyBusy(t *testing.T) {
	s := newSandbox(t)
	(&fakeExec{}).install(t)
	os.MkdirAll(filepath.Dir(s.keysFile()), 0o755)
	os.WriteFile(s.keysFile(), []byte(`{"global":{"ctrl-shift-y":"layout.switch"}}`), 0o644)
	out := must(t, "mod", "install", "--keys")
	if readKeys(t, s)["global"]["ctrl-shift-y"] != "layout.switch" {
		t.Fatal("overwrote a binding in use")
	}
	if !strings.Contains(out, "in use") {
		t.Fatalf("no busy message: %q", out)
	}
	must(t, "mod", "uninstall")
	if readKeys(t, s)["global"]["ctrl-shift-y"] != "layout.switch" {
		t.Fatal("uninstall removed a binding it did not add")
	}
}

func TestModInstallLeavesSymlinkAlone(t *testing.T) {
	s := newSandbox(t)
	(&fakeExec{}).install(t)
	dev := t.TempDir()
	os.MkdirAll(filepath.Dir(s.modDir()), 0o755)
	os.Symlink(dev, s.modDir())
	if _, err := cli(t, "mod", "install"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v", err)
	}
	if target, _ := os.Readlink(s.modDir()); target != dev {
		t.Fatal("symlink replaced")
	}
	if _, err := cli(t, "mod", "uninstall"); err == nil {
		t.Fatal("uninstall removed a panel it did not install")
	}
}

func TestServiceRestartRefreshesManagedPanel(t *testing.T) {
	s := newSandbox(t)
	(&fakeExec{}).install(t)
	must(t, "mod", "install")
	stale := filepath.Join(s.modDir(), "main.js")
	os.WriteFile(stale, []byte("old"), 0o644)
	must(t, "service", "restart")
	if data, _ := os.ReadFile(stale); string(data) == "old" {
		t.Fatal("managed panel not refreshed")
	}

	other := newSandbox(t)
	os.MkdirAll(other.modDir(), 0o755)
	os.WriteFile(filepath.Join(other.modDir(), "main.js"), []byte("mine"), 0o644)
	must(t, "service", "restart")
	if data, _ := os.ReadFile(filepath.Join(other.modDir(), "main.js")); string(data) != "mine" {
		t.Fatal("unmanaged panel touched")
	}
}

func TestSkillInstall(t *testing.T) {
	s := newSandbox(t)
	must(t, "skill", "install")
	data, err := os.ReadFile(filepath.Join(s.home, ".claude", "skills", "schedule-local", "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "name: schedule-local") {
		t.Fatalf("default dir: %v", err)
	}
	dir := t.TempDir()
	must(t, "skill", "install", dir)
	if _, err := os.Stat(filepath.Join(dir, "schedule-local", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
