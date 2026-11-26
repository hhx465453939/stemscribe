package webapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hhx465453939/stemscribe/internal/runctl"
)

func newHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	wd := t.TempDir()
	return New(runctl.NewService(), nil, wd), wd
}

func TestStopAndCancelRoutesReportMissingIDs(t *testing.T) {
	h, _ := newHandler(t)
	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/api/queue/q-999/stop", http.StatusNotFound},
		{http.MethodDelete, "/api/queue/q-999", http.StatusNotFound},
		{http.MethodPost, "/api/runs/r-999/cancel", http.StatusNotFound},
		{http.MethodGet, "/api/runs/r-999", http.StatusNotFound},
		{http.MethodGet, "/api/queue/q-999/stop", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.want {
			t.Errorf("%s %s = %d want %d (%s)", c.method, c.path, rec.Code, c.want, rec.Body.String())
		}
	}
}

// An output folder that cannot be written must be refused when the user picks it,
// not minutes later when the first artifact is written.
func TestSettingsRejectsUnwritableOutputDir(t *testing.T) {
	h, _ := newHandler(t)
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"output_dir":` + quote(file) + `}`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/settings", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT settings = %d want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestQueueRejectsUnwritableOutputDir(t *testing.T) {
	h, _ := newHandler(t)
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"inputs":["a.mp4"],"output_dir":` + quote(file) + `}`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/queue", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST queue = %d want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
