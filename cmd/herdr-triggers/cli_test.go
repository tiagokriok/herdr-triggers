package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sandbox struct {
	home, config, state string
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	root := t.TempDir()
	s := sandbox{home: filepath.Join(root, "home"), config: filepath.Join(root, "config"), state: filepath.Join(root, "state")}
	t.Setenv("HOME", s.home)
	t.Setenv("XDG_CONFIG_HOME", s.config)
	t.Setenv("XDG_STATE_HOME", s.state)
	t.Setenv("TRIGGERS_NO_UPDATE_CHECK", "")
	return s
}

func cli(t *testing.T, args ...string) (string, error) {
	t.Helper()
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	err := run(args)
	w.Close()
	os.Stdout = stdout
	var out bytes.Buffer
	io.Copy(&out, r)
	return out.String(), err
}

func must(t *testing.T, args ...string) string {
	t.Helper()
	out, err := cli(t, args...)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out
}

func TestKeepCommand(t *testing.T) {
	newSandbox(t)
	if out := must(t, "keep"); !strings.Contains(out, "50") {
		t.Fatalf("default keep: %q", out)
	}
	if out := must(t, "keep", "10"); !strings.Contains(out, "restart") {
		t.Fatalf("no restart hint: %q", out)
	}
	if out := must(t, "keep"); !strings.Contains(out, "10") {
		t.Fatalf("keep after set: %q", out)
	}
	must(t, "keep", "0")
	if out := must(t, "keep"); !strings.Contains(out, "unlimited") {
		t.Fatalf("keep 0: %q", out)
	}
	for _, bad := range []string{"-1", "x"} {
		if _, err := cli(t, "keep", bad); err == nil {
			t.Fatalf("keep %s accepted", bad)
		}
	}
}

func TestUpdatesCommand(t *testing.T) {
	newSandbox(t)
	if out := must(t, "updates"); !strings.Contains(out, "on") {
		t.Fatalf("default: %q", out)
	}
	if out := must(t, "updates", "off"); !strings.Contains(out, "restart") {
		t.Fatalf("no restart hint: %q", out)
	}
	if out := must(t, "updates"); !strings.Contains(out, "off") {
		t.Fatalf("after off: %q", out)
	}
	if _, err := cli(t, "updates", "maybe"); err == nil {
		t.Fatal("updates maybe accepted")
	}
}
