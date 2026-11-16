package service

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tiagokriok/herdr-triggers/internal/dispatch"
)

func fake(goos string, t *testing.T) (Deps, *[][]string) {
	var calls [][]string
	return Deps{
		GOOS: goos, Home: t.TempDir(), UID: 501, Exe: "/opt/bin/herdr-triggers",
		Env:  map[string]string{"PATH": "/opt/bin:/usr/bin", "CLAUDE_CONFIG_DIR": "/c", "TRIGGERS_NO_UPDATE_CHECK": "1"},
		Exec: func(argv []string) dispatch.Result { calls = append(calls, argv); return dispatch.Result{} },
	}, &calls
}

func TestLinux(t *testing.T) {
	d, calls := fake("linux", t)
	if Installed(d) {
		t.Fatal("installed before install")
	}
	msg, err := Install(d)
	if err != nil || !strings.Contains(msg, "enable-linger") {
		t.Fatal(msg, err)
	}
	unit, _ := os.ReadFile(filepath.Join(d.Home, ".config/systemd/user/herdr-triggers.service"))
	for _, want := range []string{"ExecStart=/opt/bin/herdr-triggers serve", "Restart=on-failure", "Environment=\"PATH=/opt/bin:/usr/bin\"", "Environment=\"CLAUDE_CONFIG_DIR=/c\"", "Environment=\"TRIGGERS_NO_UPDATE_CHECK=1\"", "WantedBy=default.target"} {
		if !strings.Contains(string(unit), want) {
			t.Errorf("missing %q in\n%s", want, unit)
		}
	}
	if !Installed(d) {
		t.Fatal("not installed")
	}
	Restart(d)
	Uninstall(d)
	want := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "--now", "herdr-triggers.service"},
		{"systemctl", "--user", "restart", "herdr-triggers.service"},
		{"systemctl", "--user", "disable", "--now", "herdr-triggers.service"},
		{"systemctl", "--user", "daemon-reload"},
	}
	if !slices.EqualFunc(*calls, want, slices.Equal) {
		t.Fatalf("%q", *calls)
	}
	if Installed(d) {
		t.Fatal("still installed")
	}
}

func TestMac(t *testing.T) {
	d, calls := fake("darwin", t)
	if _, err := Install(d); err != nil {
		t.Fatal(err)
	}
	plist, _ := os.ReadFile(filepath.Join(d.Home, "Library/LaunchAgents/dev.herdr.triggers.plist"))
	for _, want := range []string{"<string>/opt/bin/herdr-triggers</string>", "<string>serve</string>", "<key>KeepAlive</key>", "<string>/opt/bin:/usr/bin</string>"} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("missing %q", want)
		}
	}
	Restart(d)
	Uninstall(d)
	plistPath := filepath.Join(d.Home, "Library/LaunchAgents/dev.herdr.triggers.plist")
	want := [][]string{
		{"launchctl", "bootstrap", "gui/501", plistPath},
		{"launchctl", "kickstart", "-k", "gui/501/dev.herdr.triggers"},
		{"launchctl", "bootout", "gui/501/dev.herdr.triggers"},
	}
	if !slices.EqualFunc(*calls, want, slices.Equal) {
		t.Fatalf("%q", *calls)
	}
}

func TestInstallFailure(t *testing.T) {
	d, _ := fake("linux", t)
	d.Exec = func(argv []string) dispatch.Result {
		return dispatch.Result{Code: 1, Stderr: "Failed to connect to bus"}
	}
	if _, err := Install(d); err == nil || !strings.Contains(err.Error(), "Failed to connect to bus") {
		t.Fatal(err)
	}
}
