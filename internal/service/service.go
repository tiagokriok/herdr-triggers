package service

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/tiagokriok/herdr-triggers/internal/dispatch"
)

const (
	unitName = "herdr-triggers.service"
	label    = "dev.herdr.triggers"
)

var forwarded = []string{"PATH", "CLAUDE_CONFIG_DIR", "TRIGGERS_ADDR", "TRIGGERS_HC", "TRIGGERS_APP", "TRIGGERS_HERDR"}

type Deps struct {
	GOOS string
	Home string
	UID  int
	Exe  string
	Env  map[string]string
	Exec dispatch.Exec
}

func (d Deps) file() string {
	if d.GOOS == "darwin" {
		return filepath.Join(d.Home, "Library", "LaunchAgents", label+".plist")
	}
	return filepath.Join(d.Home, ".config", "systemd", "user", unitName)
}

func (d Deps) run(argv ...string) error {
	if r := d.Exec(argv); r.Code != 0 {
		return fmt.Errorf("%s: %s", strings.Join(argv, " "), strings.TrimSpace(r.Stderr+r.Stdout))
	}
	return nil
}

func unit(d Deps) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\nDescription=herdr-triggers scheduler and webhooks\nAfter=network-online.target\n\n[Service]\nExecStart=%s serve\nRestart=on-failure\nRestartSec=5\n", d.Exe)
	for _, k := range forwarded {
		if v := d.Env[k]; v != "" {
			fmt.Fprintf(&b, "Environment=\"%s=%s\"\n", k, v)
		}
	}
	b.WriteString("\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

func plist(d Deps) string {
	var env strings.Builder
	for _, k := range forwarded {
		if v := d.Env[k]; v != "" {
			fmt.Fprintf(&env, "      <key>%s</key><string>%s</string>\n", k, html.EscapeString(v))
		}
	}
	logFile := filepath.Join(d.Home, "Library", "Logs", "herdr-triggers.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key><string>%s</string>
    <key>ProgramArguments</key>
    <array>
      <string>%s</string>
      <string>serve</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>EnvironmentVariables</key>
    <dict>
%s    </dict>
    <key>StandardOutPath</key><string>%s</string>
    <key>StandardErrorPath</key><string>%s</string>
  </dict>
</plist>
`, label, html.EscapeString(d.Exe), env.String(), logFile, logFile)
}

func Installed(d Deps) bool {
	_, err := os.Stat(d.file())
	return err == nil
}

func Install(d Deps) (string, error) {
	content := unit(d)
	if d.GOOS == "darwin" {
		content = plist(d)
	}
	if err := os.MkdirAll(filepath.Dir(d.file()), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(d.file(), []byte(content), 0o644); err != nil {
		return "", err
	}
	if d.GOOS == "darwin" {
		if err := d.run("launchctl", "bootstrap", fmt.Sprintf("gui/%d", d.UID), d.file()); err != nil {
			return "", err
		}
		return "installed " + d.file() + "; logs: ~/Library/Logs/herdr-triggers.log", nil
	}
	if err := d.run("systemctl", "--user", "daemon-reload"); err != nil {
		return "", err
	}
	if err := d.run("systemctl", "--user", "enable", "--now", unitName); err != nil {
		return "", err
	}
	return "installed " + d.file() + "; logs: journalctl --user -u herdr-triggers\non a server, keep it running without a login: sudo loginctl enable-linger $USER", nil
}

func Restart(d Deps) error {
	if d.GOOS == "darwin" {
		return d.run("launchctl", "kickstart", "-k", fmt.Sprintf("gui/%d/%s", d.UID, label))
	}
	return d.run("systemctl", "--user", "restart", unitName)
}

func Uninstall(d Deps) error {
	if d.GOOS == "darwin" {
		d.run("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", d.UID, label))
	} else {
		d.run("systemctl", "--user", "disable", "--now", unitName)
	}
	if err := os.Remove(d.file()); err != nil && !os.IsNotExist(err) {
		return err
	}
	if d.GOOS != "darwin" {
		return d.run("systemctl", "--user", "daemon-reload")
	}
	return nil
}
