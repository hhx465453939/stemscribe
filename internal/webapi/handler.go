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
		id := strings.TrimPrefix(r.URL.Path, "/api/runs/")
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
