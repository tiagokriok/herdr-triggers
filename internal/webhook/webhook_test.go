package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tiagokriok/herdr-triggers/internal/app"
	"github.com/tiagokriok/herdr-triggers/internal/dispatch"
	"github.com/tiagokriok/herdr-triggers/internal/store"
)

const token = "s3cret-token-0123456789"

type env struct {
	srv     http.Handler
	db      *store.DB
	tasks   string
	mu      sync.Mutex
	prompts []string
	wg      sync.WaitGroup
}

func newEnv(t *testing.T) *env {
	dir := t.TempDir()
	db, _ := store.Open(filepath.Join(dir, "state.db"))
	t.Cleanup(func() { db.Close() })
	e := &env{db: db, tasks: filepath.Join(dir, "tasks.json")}
	hook := `{"id":"%s","title":"Alert","prompt":"investigate","cwd":"/tmp","schedule":{"type":"webhook","token":"` + token + `"},"enabled":%s}`
	tasks := []string{
		strings.Replace(strings.Replace(hook, "%s", "alert", 1), "%s", "true", 1),
		strings.Replace(strings.Replace(hook, "%s", "off", 1), "%s", "false", 1),
		`{"id":"cron","title":"C","prompt":"p","cwd":"/tmp","schedule":{"type":"cron","expr":"0 9 * * *"},"enabled":true}`,
	}
	os.WriteFile(e.tasks, []byte(`{"tasks":[`+strings.Join(tasks, ",")+`]}`), 0o644)
	a := &app.App{
		Tasks: e.tasks, DB: db, Log: func(string) {}, Notify: func(string, string) {},
		Dispatch: dispatch.Deps{
			HC: "hc", App: "herdr-app", Now: time.Now, Pause: func(time.Duration) {},
			Exec: func(argv []string) dispatch.Result {
				if argv[0] == "herdr-app" {
					var m map[string]any
					json.Unmarshal([]byte(argv[3]), &m)
					e.mu.Lock()
					e.prompts = append(e.prompts, m["task"].(string))
					e.mu.Unlock()
					return dispatch.Result{Stdout: `{"space":"sp1","terminal":"t1"}`}
				}
				return dispatch.Result{Stdout: `{"ended":true,"reason":"status","status":"done"}`}
			},
		},
	}
	e.srv = Handler(a, func(run func()) { e.wg.Go(run) })
	return e
}

func (e *env) do(method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

var bearer = map[string]string{"Authorization": "Bearer " + token}

func TestAccepted(t *testing.T) {
	e := newEnv(t)
	rec := e.do("POST", "/hooks/alert", `{"alert":"cpu"}`, bearer)
	e.wg.Wait()
	if rec.Code != 202 || !strings.Contains(rec.Body.String(), `"run":1`) {
		t.Fatal(rec.Code, rec.Body)
	}
	if len(e.prompts) != 1 || !strings.HasPrefix(e.prompts[0], "investigate\n\n") || !strings.Contains(e.prompts[0], "\n{\"alert\":\"cpu\"}\n</webhook-data-") {
		t.Fatalf("%q", e.prompts)
	}
	runs, _ := e.db.Runs("alert", 5)
	ds, _ := e.db.Deliveries("alert", 5)
	if runs[0].Status != "done" || len(ds) != 1 || ds[0].Outcome != "accepted" || ds[0].RunN != 1 || ds[0].Bytes != 15 {
		t.Fatalf("%+v %+v", runs, ds)
	}
}

func TestTokenPlaces(t *testing.T) {
	e := newEnv(t)
	if r := e.do("POST", "/hooks/alert", "", map[string]string{"X-Triggers-Token": token}); r.Code != 202 {
		t.Fatal(r.Code)
	}
	e.wg.Wait()
	if r := e.do("POST", "/hooks/alert?token="+token, "", nil); r.Code != 202 {
		t.Fatal(r.Code)
	}
	e.wg.Wait()
	if r := e.do("POST", "/hooks/alert/"+token, "", nil); r.Code != 202 {
		t.Fatal(r.Code)
	}
	e.wg.Wait()
}

func TestRejections(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		name, method, path, body string
		headers                  map[string]string
		code                     int
	}{
		{"wrong token", "POST", "/hooks/alert", "", map[string]string{"Authorization": "Bearer nope"}, 401},
		{"no token", "POST", "/hooks/alert", "", nil, 401},
		{"wrong path token", "POST", "/hooks/alert/nope", "", bearer, 401},
		{"unknown", "POST", "/hooks/nope", "", bearer, 404},
		{"disabled", "POST", "/hooks/off", "", bearer, 404},
		{"not webhook", "POST", "/hooks/cron", "", bearer, 404},
		{"GET", "GET", "/hooks/alert", "", bearer, 405},
		{"other path", "POST", "/other", "", bearer, 404},
		{"too big", "POST", "/hooks/alert", strings.Repeat("x", 65*1024), bearer, 413},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if r := e.do(c.method, c.path, c.body, c.headers); r.Code != c.code {
				t.Fatalf("got %d want %d", r.Code, c.code)
			}
		})
	}
	if len(e.prompts) != 0 {
		t.Fatal(e.prompts)
	}
	ds, _ := e.db.Deliveries("alert", 10)
	if len(ds) != 4 || ds[0].Outcome != "too_large" || ds[3].Outcome != "unauthorized" {
		t.Fatalf("%+v", ds)
	}
}

func TestBusy(t *testing.T) {
	e := newEnv(t)
	e.db.Reserve("alert", time.Now(), "working")
	if r := e.do("POST", "/hooks/alert", "", bearer); r.Code != 409 {
		t.Fatal(r.Code)
	}
	ds, _ := e.db.Deliveries("alert", 1)
	if ds[0].Outcome != "busy" {
		t.Fatal(ds)
	}
}

func TestInvalidTasks(t *testing.T) {
	e := newEnv(t)
	os.WriteFile(e.tasks, []byte("{"), 0o644)
	if r := e.do("POST", "/hooks/alert", "", bearer); r.Code != 503 {
		t.Fatal(r.Code)
	}
}
