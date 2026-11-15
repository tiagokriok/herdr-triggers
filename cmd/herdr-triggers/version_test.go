package main

import (
	"runtime/debug"
	"testing"
)

func TestVersionString(t *testing.T) {
	built := &debug.BuildInfo{Main: debug.Module{Version: "v0.3.0"}}
	devel := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}
	cases := []struct {
		name   string
		linked string
		info   *debug.BuildInfo
		want   string
	}{
		{"release build", "0.4.1", built, "0.4.1"},
		{"go install", "dev", built, "v0.3.0"},
		{"local build", "dev", devel, "dev"},
		{"no build info", "dev", nil, "dev"},
	}
	for _, c := range cases {
		if got := versionString(c.linked, c.info); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
