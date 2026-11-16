package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openKeep(t *testing.T, keep int) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.Keep = keep
	return db
}

func runNumbers(t *testing.T, db *DB, task string) []int {
	t.Helper()
	runs, err := db.Runs(task, 100)
	if err != nil {
		t.Fatal(err)
	}
	var ns []int
	for _, r := range runs {
		ns = append(ns, r.N)
	}
	return ns
}

func fire(t *testing.T, db *DB, task string, times int, status string) {
	t.Helper()
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	for i := range times {
		r, err := db.Reserve(task, at.Add(time.Duration(i)*time.Minute), "starting")
		if err != nil {
			t.Fatal(err)
		}
		r.Status = status
		if err := db.Save(r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestKeepPrunesOldestRunsPerTask(t *testing.T) {
	db := openKeep(t, 3)
	fire(t, db, "a", 5, "done")
	fire(t, db, "b", 2, "done")
	if got := runNumbers(t, db, "a"); len(got) != 3 || got[0] != 5 || got[2] != 3 {
		t.Fatalf("a runs = %v, want [5 4 3]", got)
	}
	if got := runNumbers(t, db, "b"); len(got) != 2 {
		t.Fatalf("b runs = %v, want untouched", got)
	}
	fire(t, db, "a", 1, "done")
	if got := runNumbers(t, db, "a"); got[0] != 6 {
		t.Fatalf("numbering restarted: %v", got)
	}
}

func TestKeepNeverPrunesActiveRuns(t *testing.T) {
	db := openKeep(t, 2)
	fire(t, db, "a", 1, "working")
	fire(t, db, "a", 3, "done")
	got := runNumbers(t, db, "a")
	if len(got) != 3 || got[2] != 1 {
		t.Fatalf("runs = %v, want active #1 kept with [4 3]", got)
	}
}

func TestKeepZeroIsUnlimited(t *testing.T) {
	db := openKeep(t, 0)
	fire(t, db, "a", 60, "done")
	if got := runNumbers(t, db, "a"); len(got) != 60 {
		t.Fatalf("kept %d runs, want 60", len(got))
	}
}

func TestKeepPrunesDeliveries(t *testing.T) {
	db := openKeep(t, 2)
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	for i := range 4 {
		db.AddDelivery(Delivery{TaskID: "a", ReceivedAt: at.Add(time.Duration(i) * time.Second), Bytes: i, Outcome: "accepted"})
	}
	db.AddDelivery(Delivery{TaskID: "b", ReceivedAt: at, Outcome: "accepted"})
	got, _ := db.Deliveries("a", 10)
	if len(got) != 2 || got[0].Bytes != 3 || got[1].Bytes != 2 {
		t.Fatalf("deliveries = %+v, want newest two", got)
	}
	if b, _ := db.Deliveries("b", 10); len(b) != 1 {
		t.Fatalf("b deliveries = %+v", b)
	}
}

func TestForgetOtherTasks(t *testing.T) {
	db := openKeep(t, 0)
	fire(t, db, "a", 2, "done")
	fire(t, db, "gone", 2, "done")
	fire(t, db, "busy", 1, "working")
	db.AddDelivery(Delivery{TaskID: "gone", ReceivedAt: time.Now(), Outcome: "accepted"})
	if err := db.ForgetExcept([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	if got := runNumbers(t, db, "busy"); len(got) != 1 {
		t.Fatalf("active run of a removed task was deleted: %v", got)
	}
	if got := runNumbers(t, db, "gone"); len(got) != 0 {
		t.Fatalf("gone runs = %v", got)
	}
	if d, _ := db.Deliveries("gone", 10); len(d) != 0 {
		t.Fatalf("gone deliveries = %+v", d)
	}
	if got := runNumbers(t, db, "a"); len(got) != 2 {
		t.Fatalf("a runs = %v", got)
	}
}
