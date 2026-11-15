package schedule

import (
	"testing"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/task"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func cronS(e string) task.Schedule { return task.Schedule{Type: "cron", Expr: e} }

func eq(t *testing.T, got *time.Time, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Fatalf("want nil, got %v", got)
		}
		return
	}
	if got == nil || !got.Equal(at(want)) {
		t.Fatalf("want %s, got %v", want, got)
	}
}

func TestNextRun(t *testing.T) {
	time.Local = time.UTC
	eq(t, Next(cronS("0 9 * * 1-5"), at("2026-11-14T10:00:00Z")), "2026-11-16T09:00:00Z")
	eq(t, Next(cronS("0 9 * * *"), at("2026-11-14T09:00:00Z")), "2026-11-15T09:00:00Z")
	eq(t, Next(cronS("0 0 1 * *"), at("2026-11-30T23:59:00Z")), "2026-12-01T00:00:00Z")
	eq(t, Next(cronS("0 18 L * *"), at("2026-02-01T00:00:00Z")), "2026-02-28T18:00:00Z")
	eq(t, Next(task.Schedule{Type: "once", RunAt: "2026-11-14T10:00:00-03:00"}, at("2026-11-14T12:00:00Z")), "2026-11-14T13:00:00Z")
	eq(t, Next(task.Schedule{Type: "once", RunAt: "2026-11-14T10:00:00Z"}, at("2026-11-14T10:00:00Z")), "")
	eq(t, Next(task.Schedule{Type: "webhook", Token: "x"}, at("2026-11-14T10:00:00Z")), "")
}

func TestNextRunDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	time.Local = ny
	defer func() { time.Local = time.UTC }()
	eq(t, Next(cronS("0 9 * * *"), at("2026-03-07T15:00:00Z")), "2026-03-08T13:00:00Z")
	eq(t, Next(cronS("0 9 * * *"), at("2026-03-06T15:00:00Z")), "2026-03-07T14:00:00Z")
}
