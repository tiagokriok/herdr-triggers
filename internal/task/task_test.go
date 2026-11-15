package task

import (
	"strings"
	"testing"
)

const valid = `{"tasks":[{"id":"daily","title":"Daily","prompt":"p","cwd":"/w","schedule":{"type":"cron","expr":"0 9 * * 1-5"},"enabled":true}]}`

func mustParse(t *testing.T, s string) File {
	t.Helper()
	f, err := Parse([]byte(s))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func parseErr(t *testing.T, s string) string {
	t.Helper()
	_, err := Parse([]byte(s))
	if err == nil {
		t.Fatalf("expected error for %s", s)
	}
	return err.Error()
}

func withTask(field string) string {
	return strings.Replace(valid, `"enabled":true`, `"enabled":true,`+field, 1)
}

func TestDefaults(t *testing.T) {
	task := mustParse(t, valid).Tasks[0]
	if task.Agent != "claude" || task.CatchUpMinutes == nil || *task.CatchUpMinutes != 120 {
		t.Fatalf("defaults: %+v", task)
	}
}

func TestOptionalFields(t *testing.T) {
	task := mustParse(t, withTask(`"agent":"pi","model":"m","agentArgs":["-v"],"timeoutMinutes":5,"catchUpMinutes":0`)).Tasks[0]
	if task.Agent != "pi" || task.Model != "m" || task.AgentArgs[0] != "-v" || task.TimeoutMinutes != 5 || *task.CatchUpMinutes != 0 {
		t.Fatalf("%+v", task)
	}
}

func TestMissingFileShapeIsEmpty(t *testing.T) {
	if len(mustParse(t, `{"tasks":[]}`).Tasks) != 0 {
		t.Fatal("expected empty")
	}
}

func TestRejects(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"json":        {"{nope", "invalid json"},
		"unknown key": {withTask(`"bogus":1`), "bogus"},
		"id":          {strings.Replace(valid, `"daily"`, `"Daily!"`, 1), "id"},
		"cwd":         {strings.Replace(valid, `"/w"`, `"rel"`, 1), "absolute"},
		"6 fields":    {strings.Replace(valid, `0 9 * * 1-5`, `0 0 9 * * 1-5`, 1), "5 fields"},
		"bad cron":    {strings.Replace(valid, `0 9 * * 1-5`, `99 * * * *`, 1), "cron"},
		"once offset": {strings.Replace(valid, `{"type":"cron","expr":"0 9 * * 1-5"}`, `{"type":"once","runAt":"2026-11-14T10:00:00"}`, 1), "runAt"},
		"type":        {strings.Replace(valid, `"cron","expr"`, `"weekly","expr"`, 1), "schedule type"},
		"short token": {strings.Replace(valid, `{"type":"cron","expr":"0 9 * * 1-5"}`, `{"type":"webhook","token":"short"}`, 1), "token"},
		"timeout":     {withTask(`"timeoutMinutes":-1`), "timeoutMinutes"},
		"title":       {strings.Replace(valid, `"Daily"`, `""`, 1), "title"},
		"dup":         {`{"tasks":[` + valid[10:len(valid)-2] + `,` + valid[10:len(valid)-2] + `]}`, "duplicate"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := parseErr(t, c.in); !strings.Contains(got, c.want) {
				t.Fatalf("error %q lacks %q", got, c.want)
			}
		})
	}
}

func TestOnceAndWebhookAccepted(t *testing.T) {
	once := strings.Replace(valid, `{"type":"cron","expr":"0 9 * * 1-5"}`, `{"type":"once","runAt":"2026-11-14T10:00:00-03:00"}`, 1)
	mustParse(t, once)
	hook := strings.Replace(valid, `{"type":"cron","expr":"0 9 * * 1-5"}`, `{"type":"webhook","token":"0123456789abcdef"}`, 1)
	mustParse(t, hook)
}
