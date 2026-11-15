package view

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
)

func TestDescribe(t *testing.T) {
	time.Local = time.UTC
	cases := map[string]task.Schedule{
		"daily at 09:00":            {Type: "cron", Expr: "0 9 * * *"},
		"weekdays at 18:30":         {Type: "cron", Expr: "30 18 * * 1-5"},
		"Mondays at 08:00":          {Type: "cron", Expr: "0 8 * * 1"},
		"monthly on day 1 at 00:00": {Type: "cron", Expr: "0 0 1 * *"},
		"hourly at :15":             {Type: "cron", Expr: "15 * * * *"},
		"every 10 min":              {Type: "cron", Expr: "*/10 * * * *"},
		"cron 0 9 1-7 * 1":          {Type: "cron", Expr: "0 9 1-7 * 1"},
		"once on 2026-11-14 10:05":  {Type: "once", RunAt: "2026-11-14T10:05:00Z"},
		"on webhook":                {Type: "webhook", Token: "x"},
	}
	for want, s := range cases {
		if got := Describe(s); got != want {
			t.Errorf("%+v: got %q want %q", s, got, want)
		}
	}
}

func TestList(t *testing.T) {
	time.Local = time.UTC
	dir := t.TempDir()
	tasks := filepath.Join(dir, "tasks.json")
	os.WriteFile(tasks, []byte(`{"tasks":[
{"id":"daily","title":"Daily","prompt":"p","cwd":"/tmp","schedule":{"type":"cron","expr":"0 9 * * *"},"enabled":true,"agent":"pi","model":"m1"},
{"id":"paused","title":"Paused","prompt":"p","cwd":"/tmp","schedule":{"type":"cron","expr":"30 18 * * 1-5"},"enabled":false}]}`), 0o644)
	db, _ := store.Open(filepath.Join(dir, "state.db"))
	defer db.Close()
	r, _ := db.Reserve("daily", time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC), "blocked")
	r.Terminal, r.Space = "t2", "sp2"
	db.Save(r)
	got, _ := json.Marshal(List(tasks, db, time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)))
	want := `{"tasks":[{"id":"daily","title":"Daily","schedule":"daily at 09:00","enabled":true,"agent":"pi","model":"m1","next":"2026-11-15T09:00:00Z","last":{"n":1,"status":"blocked","firedAt":"2026-11-14T09:00:00Z","space":"sp2","terminal":"t2"}},` +
		`{"id":"paused","title":"Paused","schedule":"weekdays at 18:30","enabled":false,"agent":"claude","model":"","next":null,"last":null}]}`
	if string(got) != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestListInvalid(t *testing.T) {
	tasks := filepath.Join(t.TempDir(), "tasks.json")
	os.WriteFile(tasks, []byte("{bad"), 0o644)
	got := List(tasks, nil, time.Now())
	if len(got.Tasks) != 0 || got.Error == "" {
		t.Fatal(got)
	}
}
