package agents

import "github.com/tiagokriok/herdr-triggers/internal/task"

type profile struct {
	modelFlag    string
	promptPrefix []string
	claudeConfig bool
}

var profiles = map[string]profile{
	"claude":   {"--model", []string{"--"}, true},
	"codex":    {"-m", []string{"--"}, false},
	"pi":       {"--model", []string{"--"}, false},
	"opencode": {"--model", []string{"--prompt"}, false},
}

var generic = profile{modelFlag: "--model"}

func Known() []string { return []string{"claude", "codex", "pi", "opencode"} }

func EnvArgv(configDir string, argv []string) []string {
	if configDir == "" {
		return argv
	}
	return append([]string{"env", "CLAUDE_CONFIG_DIR=" + configDir}, argv...)
}

func Argv(t task.Task, configDir string) []string {
	p, ok := profiles[t.Agent]
	if !ok {
		p = generic
	}
	argv := []string{t.Agent}
	if t.Model != "" {
		argv = append(argv, p.modelFlag, t.Model)
	}
	argv = append(argv, t.AgentArgs...)
	argv = append(argv, p.promptPrefix...)
	if !p.claudeConfig {
		return argv
	}
	if configDir == "" {
		configDir = t.ClaudeConfigDir
	}
	return EnvArgv(configDir, argv)
}
