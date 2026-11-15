package webhook

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/app"
	"github.com/tiagokriok/herdr-triggers/internal/store"
	"github.com/tiagokriok/herdr-triggers/internal/task"
)

const MaxBody = 64 << 10

func reply(w http.ResponseWriter, code int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, code int, msg string) {
	reply(w, code, map[string]any{"error": msg})
}

func tokenOf(r *http.Request) string {
	if t := r.PathValue("token"); t != "" {
		return t
	}
	if auth, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(auth)
	}
	if t := r.Header.Get("X-Triggers-Token"); t != "" {
		return t
	}
	return r.URL.Query().Get("token")
}

func sameToken(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func findHook(tasks []task.Task, id string) (task.Task, bool) {
	for _, t := range tasks {
		if t.ID == id && t.Enabled && t.Schedule.Type == "webhook" {
			return t, true
		}
	}
	return task.Task{}, false
}

func Handler(a *app.App, spawn func(func())) http.Handler {
	mux := http.NewServeMux()
	handle := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			fail(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		id := r.PathValue("id")
		f, err := store.LoadTasks(a.Tasks)
		if err != nil {
			fail(w, http.StatusServiceUnavailable, "tasks.json invalid")
			return
		}
		t, ok := findHook(f.Tasks, id)
		if !ok {
			fail(w, http.StatusNotFound, "no enabled webhook task "+id)
			return
		}
		record := func(bytes int, outcome string, n int) {
			a.DB.AddDelivery(store.Delivery{TaskID: id, ReceivedAt: time.Now(), Bytes: bytes, Outcome: outcome, RunN: n})
			a.Log("webhook " + id + " " + outcome)
		}
		if !sameToken(tokenOf(r), t.Schedule.Token) {
			record(0, "unauthorized", 0)
			fail(w, http.StatusUnauthorized, "bad token")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBody))
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			record(MaxBody, "too_large", 0)
			fail(w, http.StatusRequestEntityTooLarge, "body over 64KB")
			return
		}
		if err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		run, start, err := a.Begin(t, string(body))
		if errors.Is(err, app.ErrBusy) {
			record(len(body), "busy", 0)
			fail(w, http.StatusConflict, err.Error())
			return
		}
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		record(len(body), "accepted", run.N)
		spawn(func() { start() })
		reply(w, http.StatusAccepted, map[string]any{"task": id, "run": run.N})
	}
	mux.HandleFunc("/hooks/{id}", handle)
	mux.HandleFunc("/hooks/{id}/{token}", handle)
	return mux
}
