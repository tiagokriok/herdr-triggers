package app

import (
	"errors"
	"testing"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/update"
)

func TestStepChecksUpdatesOncePerDay(t *testing.T) {
	e := newEnv(t).at("2026-11-15T12:00:00Z")
	e.write(cronJSON)
	fetches := 0
	e.app.Version, e.app.UpdateCheck = "0.1.0", true
	e.app.FetchLatest = func() (string, error) { fetches++; return "v0.2.0", nil }
	s := e.step(nil)
	s = e.step(s)
	if fetches != 1 {
		t.Fatalf("fetched %d times in one day", fetches)
	}
	if info := update.Available(e.app.DB, "0.1.0", "/x", func(string) string { return "" }); info == nil || info.Latest != "v0.2.0" {
		t.Fatalf("latest not stored: %+v", info)
	}
	e.now = e.now.Add(25 * time.Hour)
	e.step(s)
	if fetches != 2 {
		t.Fatalf("fetched %d times after a day", fetches)
	}
}

func TestStepUpdateCheckOffOrFailing(t *testing.T) {
	e := newEnv(t).at("2026-11-15T12:00:00Z")
	e.write(cronJSON)
	called := false
	e.app.Version, e.app.UpdateCheck = "0.1.0", false
	e.app.FetchLatest = func() (string, error) { called = true; return "v9.9.9", nil }
	e.step(nil)
	if called {
		t.Fatal("checked while disabled")
	}
	e.app.UpdateCheck = true
	e.app.FetchLatest = func() (string, error) { return "", errors.New("offline") }
	e.step(nil)
	if update.Available(e.app.DB, "0.1.0", "/x", func(string) string { return "" }) != nil {
		t.Fatal("a failed fetch produced an update")
	}
	if update.Due(e.app.DB, e.now.Add(time.Hour)) {
		t.Fatal("a failed fetch must wait a day before retrying")
	}
}
