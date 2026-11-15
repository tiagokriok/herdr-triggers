package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPaths(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	p, err := ResolvePaths(env(map[string]string{"XDG_CONFIG_HOME": "/c", "XDG_STATE_HOME": "/s", "HOME": "/h"}))
	if err != nil || p != (Paths{Tasks: "/c/herdr-triggers/tasks.json", CLI: "/c/herdr-triggers/cli.json", Config: "/c/herdr-triggers/config.json", DB: "/s/herdr-triggers/state.db", Scheduler: "/s/herdr-triggers/scheduler.json"}) {
		t.Fatal(p, err)
	}
	p, _ = ResolvePaths(env(map[string]string{"HOME": "/h"}))
	if p.Tasks != "/h/.config/herdr-triggers/tasks.json" || p.DB != "/h/.local/state/herdr-triggers/state.db" {
		t.Fatal(p)
	}
	if _, err := ResolvePaths(env(map[string]string{})); err == nil || !strings.Contains(err.Error(), "HOME") {
		t.Fatal(err)
	}
}

const tasksJSON = `{"tasks":[
 {"id":"a","title":"A","prompt":"p","cwd":"/tmp","schedule":{"type":"once","runAt":"2026-11-14T10:00:00Z"},"enabled":true,"model":"kept"},
 {"id":"b","title":"B","prompt":"p","cwd":"/tmp","schedule":{"type":"once","runAt":"2026-11-14T10:00:00Z"},"enabled":true}]}`

func TestTasksFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tasks.json")
	if f, err := LoadTasks(filepath.Join(dir, "nope", "tasks.json")); err != nil || len(f.Tasks) != 0 {
		t.Fatal(f, err)
	}
	os.WriteFile(file, []byte(tasksJSON), 0o644)
	if err := SetEnabled(file, "a", false); err != nil {
		t.Fatal(err)
	}
	if err := SetEnabled(file, "zz", true); err == nil || !strings.Contains(err.Error(), "unknown task: zz") {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(file)
	var saved struct{ Tasks []map[string]any }
	json.Unmarshal(raw, &saved)
	if saved.Tasks[0]["enabled"] != false || saved.Tasks[1]["enabled"] != true || saved.Tasks[0]["model"] != "kept" {
		t.Fatal(string(raw))
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestSaveTask(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tasks.json")
	task := map[string]any{"id": "n", "title": "N", "prompt": "p", "cwd": "/tmp", "schedule": map[string]any{"type": "cron", "expr": "0 9 * * *"}, "enabled": true}
	if err := SaveTask(file, task); err != nil {
		t.Fatal(err)
	}
	task["title"] = "N2"
	if err := SaveTask(file, task); err != nil {
		t.Fatal(err)
	}
	f, err := LoadTasks(file)
	if err != nil || len(f.Tasks) != 1 || f.Tasks[0].Title != "N2" {
		t.Fatal(f, err)
	}
	bad := map[string]any{"id": "x", "title": "X", "prompt": "p", "cwd": "rel", "schedule": map[string]any{"type": "cron", "expr": "0 9 * * *"}, "enabled": true}
	if err := SaveTask(file, bad); err == nil {
		t.Fatal("invalid task saved")
	}
	if err := DeleteTask(file, "n"); err != nil {
		t.Fatal(err)
	}
	if f, _ := LoadTasks(file); len(f.Tasks) != 0 {
		t.Fatal(f)
	}
}

func openDB(t *testing.T) *DB {
	db, err := Open(filepath.Join(t.TempDir(), "state", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRuns(t *testing.T) {
	db := openDB(t)
	at := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	a1, _ := db.Reserve("a", at, "starting")
	b1, _ := db.Reserve("b", at, "starting")
	a2, _ := db.Reserve("a", at.Add(time.Hour), "missed")
	if a1.N != 1 || b1.N != 1 || a2.N != 2 {
		t.Fatal(a1, b1, a2)
	}
	a1.Status, a1.Terminal, a1.Space = "working", "t1", "sp1"
	timeout := at.Add(time.Minute)
	a1.TimeoutAt = &timeout
	if err := db.Save(a1); err != nil {
		t.Fatal(err)
	}
	last, _ := db.Last("a")
	if last == nil || last.N != 2 || last.Status != "missed" {
		t.Fatal(last)
	}
	active, _ := db.Active()
	if len(active) != 2 {
		t.Fatal(active)
	}
	runs, _ := db.Runs("a", 10)
	if len(runs) != 2 || runs[0].N != 2 || runs[1].Terminal != "t1" || !runs[1].TimeoutAt.Equal(timeout) {
		t.Fatalf("%+v", runs)
	}
	if none, _ := db.Last("zz"); none != nil {
		t.Fatal(none)
	}
}

func TestReserveConcurrent(t *testing.T) {
	db := openDB(t)
	var wg sync.WaitGroup
	ns := make(chan int, 20)
	for range 20 {
		wg.Go(func() {
			r, err := db.Reserve("a", time.Now(), "starting")
			if err != nil {
				t.Error(err)
			}
			ns <- r.N
		})
	}
	wg.Wait()
	close(ns)
	seen := map[int]bool{}
	for n := range ns {
		if seen[n] {
			t.Fatalf("duplicate n %d", n)
		}
		seen[n] = true
	}
}

func TestDeliveries(t *testing.T) {
	db := openDB(t)
	d := Delivery{TaskID: "a", ReceivedAt: time.Now(), Bytes: 12, Outcome: "accepted", RunN: 3}
	if err := db.AddDelivery(d); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Deliveries("a", 5)
	if len(got) != 1 || got[0].Outcome != "accepted" || got[0].RunN != 3 {
		t.Fatalf("%+v", got)
	}
}

func TestMeta(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "state.db"))
	defer db.Close()
	if v := db.Meta("public_url"); v != "" {
		t.Fatal(v)
	}
	db.SetMeta("public_url", "https://a")
	db.SetMeta("public_url", "https://b")
	if v := db.Meta("public_url"); v != "https://b" {
		t.Fatal(v)
	}
	db.SetMeta("public_url", "")
	if v := db.Meta("public_url"); v != "" {
		t.Fatal(v)
	}
}

func TestTaskCache(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tasks.json")
	c := &TaskCache{File: file}
	if f, err := c.Load(); err != nil || len(f.Tasks) != 0 {
		t.Fatal(f, err)
	}
	one := `{"tasks":[{"id":"a","title":"A","prompt":"p","cwd":"/tmp","schedule":{"type":"cron","expr":"0 9 * * *"},"enabled":true}]}`
	os.WriteFile(file, []byte(one), 0o644)
	if f, err := c.Load(); err != nil || len(f.Tasks) != 1 {
		t.Fatal(f, err)
	}
	st, _ := os.Stat(file)
	os.WriteFile(file, []byte(strings.Replace(one, `true`, `ttue`, 1)), 0o644)
	os.Chtimes(file, st.ModTime(), st.ModTime())
	if f, err := c.Load(); err != nil || f.Tasks[0].Title != "A" {
		t.Fatal("re-read an unchanged file", f, err)
	}
	os.Chtimes(file, st.ModTime().Add(time.Second), st.ModTime().Add(time.Second))
	if _, err := c.Load(); err == nil {
		t.Fatal("changed file not re-read")
	}
	os.Remove(file)
	if f, err := c.Load(); err != nil || len(f.Tasks) != 0 {
		t.Fatal("removed file", f, err)
	}
}
