// Package webapi is the HTTP control plane for runs.
//
// It exists as a plain http.Handler on purpose: the same value is mounted in the
// browser shell (loopback server) and in the native window (Wails
// AssetServer.Handler). Two shells, one implementation -- otherwise the window
// path would drift from the browser path and only one of them would be tested
// (ADR-022).
package webapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/engine"
	"github.com/hhx465453939/stemscribe/internal/runctl"
)

// RunRequest is the body of POST /api/runs.
type RunRequest struct {
	Input  string            `json:"input"`
	Title  string            `json:"title"`
	Model  string            `json:"model"`
	Params map[string]string `json:"params"`
	// Separate is opt-in and currently reports unavailability rather than failing.
	Separate bool `json:"separate"`
}

// New builds the handler. Paths under /api/ are handled here; everything else is
// delegated to fallback, so a single mux can serve both API and static assets.
func New(svc *runctl.Service, fallback http.Handler, workdir string) http.Handler {
	mux := http.NewServeMux()
	// One installer per handler: installs are serialised, because a second concurrent
	// download of the same artifact would corrupt the shared .part file.
	inst := runctl.NewInstaller()
	// The queue shares the same runner as single runs, so progress and results come
	// from one place (ADR-027).
	q := runctl.NewQueue(svc)
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
			return
		}
		var req RunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		if strings.TrimSpace(req.Input) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
			return
		}
		params, err := ParamsFromMap(req.Params)
		if err != nil {
			// Reject rather than ignore: silently dropping a knob the user set is the
			// same lie as a silent downgrade.
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		id, err := svc.Start(runctl.Options{
			Input:      req.Input,
			Title:      req.Title,
			ModelID:    req.Model,
			Engine:     "auto",
			DurationMS: 30000,
			Params:     params,
			Separate:   req.Separate,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if err == runctl.ErrBusy {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
	})

	mux.HandleFunc("/api/runs/latest", func(w http.ResponseWriter, r *http.Request) {
		st, ok := svc.Latest()
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no runs yet"})
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	mux.HandleFunc("/api/runs/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/runs/")
		if id, ok := strings.CutSuffix(rest, "/cancel"); ok {
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
				return
			}
			switch err := svc.Cancel(id); err {
			case nil:
				writeJSON(w, http.StatusAccepted, map[string]string{"state": "stopping"})
			case runctl.ErrRunNotFound:
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			case runctl.ErrNotRunning:
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}
		id := rest
		if id == "" || strings.Contains(id, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad run id"})
			return
		}
		st, ok := svc.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown run " + id})
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	mux.HandleFunc("/api/library", func(w http.ResponseWriter, r *http.Request) {
		items, err := runctl.Library(workdir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	mux.HandleFunc("/api/models", func(w http.ResponseWriter, r *http.Request) {
		cat, err := runctl.CatalogNow(workdir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, cat)
	})

	mux.HandleFunc("/api/models/install", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, inst.Status())
	})

	mux.HandleFunc("/api/models/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/models/")
		id, ok := strings.CutSuffix(rest, "/install")
		if !ok || id == "" || strings.Contains(id, "/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown path"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
			return
		}
		jobID, err := inst.Start(workdir, id)
		if err != nil {
			status := http.StatusInternalServerError
			if err == runctl.ErrInstallBusy {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"id": jobID})
	})

	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		settings, layout, err := runctl.LoadSettings(workdir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{
				"output_dir": settings.OutputDir,
				"model":      settings.Stages.Whisper,
				"separator":  settings.Stages.Separator,
				"workdir":    settings.Workdir,
			})
		case http.MethodPut:
			var body struct {
				OutputDir *string `json:"output_dir"`
				Model     *string `json:"model"`
				Separator *string `json:"separator"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
				return
			}
			if body.OutputDir != nil {
				// Refuse a path that cannot be created or written rather than failing at
				// run time with separation already done. MkdirAll alone is not enough on
				// Windows: an existing read-only folder passes it and fails on first write.
				dir := strings.TrimSpace(*body.OutputDir)
				if mkErr := runctl.CheckOutputDir(dir); mkErr != nil {
					// CheckOutputDir's message already says what is wrong and why; adding a
					// second prefix produced "cannot use that folder: cannot use that folder".
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": mkErr.Error()})
					return
				}
				settings.OutputDir = dir
			}
			if body.Model != nil {
				settings.Stages.Whisper = *body.Model
			}
			if body.Separator != nil {
				settings.Stages.Separator = *body.Separator
			}
			if err := layout.Save(settings); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"output_dir": settings.OutputDir, "model": settings.Stages.Whisper})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use GET or PUT"})
		}
	})

	mux.HandleFunc("/api/queue", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, q.Snapshot())
		case http.MethodPost:
			var req struct {
				Inputs    []string `json:"inputs"`
				OutputDir string   `json:"output_dir"`
				Model     string   `json:"model"`
				// Separator overrides the configured separation model for these
				// items, chosen on the New task page (stage 1).
				Separator string            `json:"separator"`
				Stems     []string          `json:"stems"`
				Params    map[string]string `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
				return
			}
			params, err := ParamsFromMap(req.Params)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if err := runctl.CheckOutputDir(req.OutputDir); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			added, rejected := q.Enqueue(req.Inputs, runctl.Options{
				ModelID:     req.Model,
				SeparatorID: req.Separator,
				ExportStems: req.Stems,
				Params:      params,
				OutputDir:   req.OutputDir,
				Engine:      "auto",
				DurationMS:  30000,
			})
			writeJSON(w, http.StatusAccepted, map[string]any{"added": len(added), "total": q.Snapshot().Total, "rejected": rejected})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use GET or POST"})
		}
	})

	mux.HandleFunc("/api/queue/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/queue/")
		// POST /api/queue/{id}/stop cancels the running item; DELETE is still the
		// "remove a waiting item" verb and stays untouched.
		if id, ok := strings.CutSuffix(rest, "/stop"); ok {
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
				return
			}
			if id == "" || strings.Contains(id, "/") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown queue item"})
				return
			}
			switch err := q.Stop(id); err {
			case nil:
				writeJSON(w, http.StatusAccepted, map[string]string{"state": "stopping"})
			case runctl.ErrQueueNotRunning:
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			case runctl.ErrQueueNotFound:
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}
		id := rest
		if r.Method != http.MethodDelete || id == "" {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use DELETE /api/queue/{id}"})
			return
		}
		switch err := q.Remove(id); err {
		case nil:
			w.WriteHeader(http.StatusNoContent)
		case runctl.ErrQueueRunning:
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		}
	})

	mux.HandleFunc("/api/capabilities", func(w http.ResponseWriter, r *http.Request) {
		// Computed, not asserted: the parameters page greys out exactly these keys, so a
		// hardcoded true/false here would decide what the user is allowed to touch.
		caps, err := runctl.CapabilitiesNow(workdir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, caps)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			mux.ServeHTTP(w, r)
			return
		}
		if fallback != nil {
			fallback.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ParamsFromMap converts the web keys (engine.Specs() keys) into the engine struct.
//
// Only the knobs the UI exposes today are accepted. An unknown key is an error
// rather than a no-op: a control that looks like it works and does nothing is worse
// than a control that refuses.
func ParamsFromMap(m map[string]string) (engine.WhisperParams, error) {
	p := engine.Defaults()
	for k, v := range m {
		switch k {
		case "input_language":
			p.InputLanguage = v
		case "translate":
			p.Translate = v == "true"
		case "detect_only":
			p.DetectOnly = v == "true"
		case "beam_size":
			n, err := atoi(v)
			if err != nil {
				return p, err
			}
			p.BeamSize = n
		case "threads":
			n, err := atoi(v)
			if err != nil {
				return p, err
			}
			p.Threads = n
		case "vad":
			p.VAD = v == "true"
		case "max_len":
			// Line length drives whether the output is readable as subtitles. Accepted here
			// so the UI baseline and an explicit user choice travel the same validated path.
			n, err := atoi(v)
			if err != nil {
				return p, err
			}
			p.MaxLen = n
		case "split_on_word":
			p.SplitOnWord = v == "true"
		case "initial_prompt":
			p.InitialPrompt = v
		default:
			return p, &UnknownParam{Key: k}
		}
	}
	return p, nil
}

// UnknownParam names the offending key so the client can fix it.
type UnknownParam struct{ Key string }

func (e *UnknownParam) Error() string { return "unknown parameter: " + e.Key }

func atoi(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, &UnknownParam{Key: "empty number"}
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, &UnknownParam{Key: "not a number: " + s}
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
