package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ActiveStatuses = []string{"starting", "working", "blocked"}

type Run struct {
	TaskID    string     `json:"taskId"`
	N         int        `json:"n"`
	FiredAt   time.Time  `json:"firedAt"`
	Terminal  string     `json:"terminal,omitempty"`
	Space     string     `json:"space,omitempty"`
	TimeoutAt *time.Time `json:"timeoutAt,omitempty"`
	Status    string     `json:"status"`
	Error     string     `json:"error,omitempty"`
}

func (r Run) Active() bool {
	for _, s := range ActiveStatuses {
		if r.Status == s {
			return true
		}
	}
	return false
}

type Delivery struct {
	TaskID     string    `json:"taskId"`
	ReceivedAt time.Time `json:"receivedAt"`
	Bytes      int       `json:"bytes"`
	Outcome    string    `json:"outcome"`
	RunN       int       `json:"runN,omitempty"`
}

type DB struct{ sql *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS runs (
  task_id TEXT NOT NULL,
  n INTEGER NOT NULL,
  fired_at TEXT NOT NULL,
  terminal TEXT NOT NULL DEFAULT '',
  space TEXT NOT NULL DEFAULT '',
  timeout_at TEXT,
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (task_id, n)
);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE TABLE IF NOT EXISTS deliveries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id TEXT NOT NULL,
  received_at TEXT NOT NULL,
  bytes INTEGER NOT NULL,
  outcome TEXT NOT NULL,
  run_n INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_deliveries_task ON deliveries(task_id, received_at);
`

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{db}, nil
}

func (d *DB) Close() error { return d.sql.Close() }

const stamp = time.RFC3339Nano

func (d *DB) Reserve(taskID string, firedAt time.Time, status string) (Run, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(n), 0) + 1 FROM runs WHERE task_id = ?`, taskID).Scan(&n); err != nil {
		return Run{}, err
	}
	r := Run{TaskID: taskID, N: n, FiredAt: firedAt.UTC(), Status: status}
	if _, err := tx.Exec(`INSERT INTO runs (task_id, n, fired_at, status) VALUES (?, ?, ?, ?)`, taskID, n, r.FiredAt.Format(stamp), status); err != nil {
		return Run{}, err
	}
	return r, tx.Commit()
}

func (d *DB) Save(r Run) error {
	var timeout any
	if r.TimeoutAt != nil {
		timeout = r.TimeoutAt.UTC().Format(stamp)
	}
	_, err := d.sql.Exec(`INSERT INTO runs (task_id, n, fired_at, terminal, space, timeout_at, status, error) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (task_id, n) DO UPDATE SET terminal = excluded.terminal, space = excluded.space, timeout_at = excluded.timeout_at, status = excluded.status, error = excluded.error`,
		r.TaskID, r.N, r.FiredAt.UTC().Format(stamp), r.Terminal, r.Space, timeout, r.Status, r.Error)
	return err
}

const runCols = `task_id, n, fired_at, terminal, space, timeout_at, status, error`

func scanRuns(rows *sql.Rows, err error) ([]Run, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var r Run
		var fired string
		var timeout sql.NullString
		if err := rows.Scan(&r.TaskID, &r.N, &fired, &r.Terminal, &r.Space, &timeout, &r.Status, &r.Error); err != nil {
			return nil, err
		}
		r.FiredAt, _ = time.Parse(stamp, fired)
		if timeout.Valid {
			t, _ := time.Parse(stamp, timeout.String)
			r.TimeoutAt = &t
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (d *DB) Runs(taskID string, limit int) ([]Run, error) {
	return scanRuns(d.sql.Query(`SELECT `+runCols+` FROM runs WHERE task_id = ? ORDER BY n DESC LIMIT ?`, taskID, limit))
}

func (d *DB) Recent(limit int) ([]Run, error) {
	return scanRuns(d.sql.Query(`SELECT `+runCols+` FROM runs ORDER BY fired_at DESC LIMIT ?`, limit))
}

func (d *DB) Last(taskID string) (*Run, error) {
	runs, err := d.Runs(taskID, 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}

func (d *DB) Active() ([]Run, error) {
	return scanRuns(d.sql.Query(`SELECT ` + runCols + ` FROM runs WHERE status IN ('starting', 'working', 'blocked') ORDER BY fired_at`))
}

func (d *DB) HasRun(taskID string) (bool, error) {
	last, err := d.Last(taskID)
	return last != nil, err
}

func (d *DB) AddDelivery(x Delivery) error {
	_, err := d.sql.Exec(`INSERT INTO deliveries (task_id, received_at, bytes, outcome, run_n) VALUES (?, ?, ?, ?, ?)`,
		x.TaskID, x.ReceivedAt.UTC().Format(stamp), x.Bytes, x.Outcome, x.RunN)
	return err
}

func (d *DB) Deliveries(taskID string, limit int) ([]Delivery, error) {
	rows, err := d.sql.Query(`SELECT task_id, received_at, bytes, outcome, run_n FROM deliveries WHERE task_id = ? ORDER BY id DESC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Delivery{}
	for rows.Next() {
		var x Delivery
		var at string
		if err := rows.Scan(&x.TaskID, &at, &x.Bytes, &x.Outcome, &x.RunN); err != nil {
			return nil, err
		}
		x.ReceivedAt, _ = time.Parse(stamp, at)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (d *DB) SetMeta(key, value string) error {
	if value == "" {
		_, err := d.sql.Exec(`DELETE FROM meta WHERE key = ?`, key)
		return err
	}
	_, err := d.sql.Exec(`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (d *DB) Meta(key string) string {
	var v string
	d.sql.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	return v
}
