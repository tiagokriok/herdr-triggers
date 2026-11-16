package app

import (
	"os"
	"testing"
)

func TestStepForgetsRemovedTasks(t *testing.T) {
	e := newEnv(t).at("2026-11-14T08:00:00Z")
	e.app.DB.Reserve("daily", e.now, "done")
	e.app.DB.Reserve("gone", e.now, "done")
	e.write(cronJSON)
	s := e.step(nil)
	if len(e.runs("gone")) != 0 {
		t.Fatal("history of a task missing from tasks.json was kept")
	}
	if len(e.runs("daily")) != 1 {
		t.Fatal("history of a present task was deleted")
	}

	e.app.DB.Reserve("gone", e.now, "done")
	os.WriteFile(e.tasks, []byte(`{"tasks":[ broken`), 0o644)
	e.step(s)
	if len(e.runs("gone")) != 1 || len(e.runs("daily")) != 1 {
		t.Fatal("an invalid tasks.json deleted history")
	}
}
