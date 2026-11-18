package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/registry"
)

// Installed describes a runtime found on disk.
type Installed struct {
	Version string
	Backend registry.Backend
	Entry   string // absolute path to whisper-cli.exe
	Root    string
}

// Missing enumerates everything needed before a real run can start.
//
// Collected in one pass on purpose: reporting one missing item at a time forces
// the user (or CI) into a fix-run-fix loop for what is really a single
// "environment not ready" state (AC-10).
type Missing struct {
	Runtime string
	Models  []string
	FFmpeg  bool
}

// Empty reports whether the environment is ready to run.
func (m Missing) Empty() bool {
	return m.Runtime == "" && len(m.Models) == 0 && !m.FFmpeg
}

// Hints renders one actionable line per missing item, in install order.
func (m Missing) Hints() []string {
	var out []string
	if m.Runtime != "" {
		out = append(out, fmt.Sprintf(
			"missing runtime: %s\n    install: stemscribe runtime install %s", m.Runtime, m.Runtime))
	}
	for _, id := range m.Models {
		out = append(out, fmt.Sprintf(
			"missing model: %s\n    install: stemscribe model install %s", id, id))
	}
	if m.FFmpeg {
		out = append(out, "missing ffmpeg\n    install: put ffmpeg.exe under <workdir>/runtimes/ffmpeg/<ver>/ or add it to PATH")
	}
	return out
}

// Err returns a single error describing every gap, or nil when ready.
func (m Missing) Err() error {
	if m.Empty() {
		return nil
	}
	return fmt.Errorf("environment not ready:\n  %s", strings.Join(m.Hints(), "\n  "))
}

// FindRuntime locates an installed whisper runtime for the given backend.
//
// Layout is fixed by MODEL-REGISTRY §4: runtimes/whisper/<version>-<backend>/.
func FindRuntime(workdir string, backend registry.Backend) (Installed, bool) {
	base := filepath.Join(workdir, "runtimes", "whisper")
	entries, err := os.ReadDir(base)
	if err != nil {
		return Installed{}, false
	}
	suffix := "-" + string(backend)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		root := filepath.Join(base, e.Name())
		entry := filepath.Join(root, "whisper-cli.exe")
		if _, statErr := os.Stat(entry); statErr != nil {
			continue // directory exists but the build is incomplete
		}
		return Installed{
			Version: strings.TrimSuffix(e.Name(), suffix),
			Backend: backend,
			Entry:   entry,
			Root:    root,
		}, true
	}
	return Installed{}, false
}

// FindModel locates an installed model by registry id.
func FindModel(workdir string, reg registry.Registry, modelID string) (string, bool) {
	m, ok := reg.FindModel(modelID)
	if !ok || len(m.Files) == 0 {
		return "", false
	}
	dest := filepath.FromSlash(m.Files[0].Dest)
	path := filepath.Join(workdir, "models", m.Family, modelID, dest)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

// FindFFmpeg looks for ffmpeg next to our runtimes, then on PATH.
//
// ffmpeg has no published asset yet, so a PATH install is accepted: it keeps the
// tool usable for people who already have ffmpeg without pretending we manage
// that copy (provenance then records a path rather than a managed runtime id).
func FindFFmpeg(workdir string) (string, bool) {
	base := filepath.Join(workdir, "runtimes", "ffmpeg")
	if entries, err := os.ReadDir(base); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			// Archives built upstream put the binary under bin/, while a manual
			// placement usually drops it at the runtime root. Accept both, otherwise a
			// correctly installed ffmpeg would be reported as missing.
			for _, rel := range []string{"ffmpeg.exe", filepath.Join("bin", "ffmpeg.exe")} {
				p := filepath.Join(base, e.Name(), rel)
				if _, statErr := os.Stat(p); statErr == nil {
					return p, true
				}
			}
		}
	}
	if p, err := exec.LookPath("ffmpeg.exe"); err == nil {
		return p, true
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, true
	}
	return "", false
}

// Diagnose reports every missing piece in one pass.
//
// ffmpeg is only reported when the input actually needs demuxing: demanding it for
// an already-16k-mono wav is a prerequisite the run does not have.
func Diagnose(workdir string, reg registry.Registry, backend registry.Backend, modelID string, needsDemux bool) Missing {
	var m Missing

	if _, ok := FindRuntime(workdir, backend); !ok {
		if rt, found := reg.FindRuntime(registry.EngineWhisper, backend, "windows-x64"); found {
			m.Runtime = rt.ID
		} else {
			m.Runtime = "whispercpp-" + string(backend) + "-win-x64"
		}
	}

	if modelID == "" {
		// No model chosen: name the cheapest usable default so the message is
		// actionable instead of "pick a model".
		m.Models = append(m.Models, "whisper-base")
	} else if _, ok := FindModel(workdir, reg, modelID); !ok {
		m.Models = append(m.Models, modelID)
	}

	if needsDemux {
		if _, ok := FindFFmpeg(workdir); !ok {
			m.FFmpeg = true
		}
	}
	return m
}
