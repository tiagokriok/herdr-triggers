package agents

import (
	"slices"
	"testing"

	"github.com/tiagokriok/herdr-triggers/internal/task"
)

func mk(agent, model string, args ...string) task.Task {
	return task.Task{Agent: agent, Model: model, AgentArgs: args}
}

func TestArgv(t *testing.T) {
	cases := []struct {
		name      string
		task      task.Task
		configDir string
		want      []string
	}{
		{"claude bare", mk("claude", ""), "", []string{"claude", "--"}},
		{"claude model+config", mk("claude", "sonnet"), "/cfg", []string{"env", "CLAUDE_CONFIG_DIR=/cfg", "claude", "--model", "sonnet", "--"}},
		{"claude task field fallback", task.Task{Agent: "claude", ClaudeConfigDir: "/p"}, "", []string{"env", "CLAUDE_CONFIG_DIR=/p", "claude", "--"}},
		{"codex ignores config dir", task.Task{Agent: "codex", ClaudeConfigDir: "/p"}, "/cfg", []string{"codex", "--"}},
		{"codex -m", mk("codex", "o3"), "", []string{"codex", "-m", "o3", "--"}},
		{"pi", mk("pi", "a/x"), "", []string{"pi", "--model", "a/x", "--"}},
		{"opencode --prompt", mk("opencode", "a/b"), "", []string{"opencode", "--model", "a/b", "--prompt"}},
		{"generic", mk("gemini", "g"), "", []string{"gemini", "--model", "g"}},
		{"agentArgs before separator", mk("claude", "", "--verbose"), "", []string{"claude", "--verbose", "--"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Argv(c.task, c.configDir); !slices.Equal(got, c.want) {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestEnvArgv(t *testing.T) {
	if got := EnvArgv("", []string{"x"}); !slices.Equal(got, []string{"x"}) {
		t.Fatal(got)
	}
}
