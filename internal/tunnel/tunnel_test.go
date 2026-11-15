package tunnel

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func fake(lines []string, err error) (Starter, *[]string, *bool) {
	var got []string
	stopped := false
	return func(argv []string) (<-chan string, func(), error) {
		got = argv
		if err != nil {
			return nil, nil, err
		}
		ch := make(chan string, len(lines))
		for _, l := range lines {
			ch <- l
		}
		close(ch)
		return ch, func() { stopped = true }, nil
	}, &got, &stopped
}

func TestOpen(t *testing.T) {
	cases := []struct {
		cfg   Config
		lines []string
		argv  []string
		url   string
	}{
		{Config{Provider: "tailscale"}, []string{"Available on the internet:", "https://box.tail1.ts.net/", "|-- proxy http://127.0.0.1:8787"}, []string{"tailscale", "funnel", "8787"}, "https://box.tail1.ts.net"},
		{Config{Provider: "cloudflared"}, []string{"INF Requesting new quick Tunnel", "INF |  https://a-b-c.trycloudflare.com  |", "INF Registered tunnel connection connIndex=0"}, []string{"cloudflared", "tunnel", "--no-autoupdate", "--protocol", "http2", "--url", "http://127.0.0.1:8787"}, "https://a-b-c.trycloudflare.com"},
		{Config{Provider: "ngrok"}, []string{`t=1 lvl=info msg="started tunnel" obj=tunnels name=command_line addr=http://127.0.0.1:8787 url=https://x1.ngrok-free.app`}, []string{"ngrok", "http", "127.0.0.1:8787", "--log", "stdout"}, "https://x1.ngrok-free.app"},
		{Config{Provider: "ngrok", Domain: "hooks.example.dev"}, []string{`msg="started tunnel" url=https://hooks.example.dev`}, []string{"ngrok", "http", "127.0.0.1:8787", "--log", "stdout", "--url", "https://hooks.example.dev"}, "https://hooks.example.dev"},
	}
	for _, c := range cases {
		t.Run(c.cfg.Provider+c.cfg.Domain, func(t *testing.T) {
			start, argv, stopped := fake(c.lines, nil)
			url, stop, err := Open(c.cfg, "127.0.0.1:8787", start, time.Second)
			if err != nil || url != c.url || !slices.Equal(*argv, c.argv) {
				t.Fatal(url, err, *argv)
			}
			stop()
			if !*stopped {
				t.Fatal("not stopped")
			}
		})
	}
}

func TestOpenWithoutProcess(t *testing.T) {
	start, argv, _ := fake(nil, nil)
	for cfg, want := range map[Config]string{
		{}:                 "http://127.0.0.1:8787",
		{Provider: "none"}: "http://127.0.0.1:8787",
		{Provider: "url", BaseURL: "https://h.example.dev/x/"}: "https://h.example.dev/x",
	} {
		url, stop, err := Open(cfg, "127.0.0.1:8787", start, time.Second)
		stop()
		if err != nil || url != want || *argv != nil {
			t.Fatal(cfg, url, err)
		}
	}
}

func TestOpenFailures(t *testing.T) {
	start, _, _ := fake(nil, errors.New("exec: \"ngrok\": not found"))
	if _, _, err := Open(Config{Provider: "ngrok"}, "127.0.0.1:8787", start, time.Second); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatal(err)
	}
	start, _, stopped := fake([]string{"error: Funnel not available; enable it in the admin console"}, nil)
	_, _, err := Open(Config{Provider: "tailscale"}, "127.0.0.1:8787", start, time.Second)
	if err == nil || !strings.Contains(err.Error(), "Funnel not available") || !*stopped {
		t.Fatal(err)
	}
	start, _, _ = fake([]string{"INF |  https://a-b-c.trycloudflare.com  |", "ERR Failed to dial"}, nil)
	if _, _, err := Open(Config{Provider: "cloudflared"}, "127.0.0.1:8787", start, time.Second); err == nil || !strings.Contains(err.Error(), "Failed to dial") {
		t.Fatal(err)
	}
	hang := func([]string) (<-chan string, func(), error) { return make(chan string), func() {}, nil }
	if _, _, err := Open(Config{Provider: "cloudflared"}, "127.0.0.1:8787", hang, 10*time.Millisecond); err == nil || !strings.Contains(err.Error(), "no public url") {
		t.Fatal(err)
	}
}
