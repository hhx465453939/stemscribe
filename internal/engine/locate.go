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
	// SeparatorRuntime marks a missing UVR5 Python environment. Separation is a
	// mandatory stage (ADR-037), so this is a hard prerequisite, not a hint.
	SeparatorRuntime bool
	SeparatorModels  []string
	FFmpeg           bool
}

// Empty reports whether the environment is ready to run.
func (m Missing) Empty() bool {
	return m.Runtime == "" && len(m.Models) == 0 && !m.SeparatorRuntime &&
		len(m.SeparatorModels) == 0 && !m.FFmpeg
}

// Hints renders one actionable line per missing item, in install order.
func (m Missing) Hints() []string {
	var out []string
	if m.Runtime != "" {
		out = append(out, fmt.Sprintf(
			"missing runtime: %s\n    install: stemscribe runtime install %s", m.Runtime, m.Runtime))
	}
	if m.SeparatorRuntime {
		out = append(out, fmt.Sprintf(
			"missing vocal-separation runtime (UVR5 via audio-separator)\n    install: stemscribe runtime install %s", SeparatorRuntimeID))
	}
	for _, id := range m.Models {
		out = append(out, fmt.Sprintf(
			"missing model: %s\n    install: stemscribe model install %s", id, id))
	}
	for _, id := range m.SeparatorModels {
		out = append(out, fmt.Sprintf(
			"missing separation model: %s\n    install: stemscribe model install %s", id, id))
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
	// The commands are for the CLI and for bug reports; a desktop user sees this text
	// in the queue row, so name the in-app path too rather than sending them to a
	// terminal they may not have open.
	return fmt.Errorf("environment not ready:\n  %s\n\n  (in the app: open the Models page to install these)", strings.Join(m.Hints(), "\n  "))
}

// suggestRuntime names the first whisper runtime an install can actually deliver.
//
// Only entries with a measured sha256 qualify: "install" refuses null+tbd bytes
// (ADR-025), so pointing at an unpublished archive would be advice that cannot be
// followed. Order follows the backend preference, then CPU is the last resort.
func suggestRuntime(reg registry.Registry, preferred registry.Backend) string {
	order := []registry.Backend{preferred, registry.BackendVulkan, registry.BackendCPU}
	seen := map[registry.Backend]bool{}
	for _, b := range order {
		if seen[b] {
			continue
		}
		seen[b] = true
		rt, ok := reg.FindRuntime(registry.EngineWhisper, b, "windows-x64")
		if !ok {
			continue
		}
		if rt.Archive.SHA256 == nil || strings.TrimSpace(*rt.Archive.SHA256) == "" {
			continue
		}
		return rt.ID
	}
	return "whispercpp-" + string(preferred) + "-win-x64"
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

// FindModel locates an installed model by registry id and returns the file the
// engine is asked to load.
//
// Every declared file must be present, not just the first one: a Demucs model is
// a YAML plus one or more .th weights, and a half-copied directory used to read
// as "installed" and then fail deep inside the Python stack with a download
// error. Checking the whole manifest makes the failure land at the prerequisite
// check, where the message can say what to install.
func FindModel(workdir string, reg registry.Registry, modelID string) (string, bool) {
	m, ok := reg.FindModel(modelID)
	if !ok || len(m.Files) == 0 {
		return "", false
	}
	dir := filepath.Join(workdir, "models", m.Family, modelID)
	for _, f := range m.Files {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f.Dest))); err != nil {
			return "", false
		}
	}
	return filepath.Join(dir, entryFile(m)), true
}

// entryFile is the manifest entry the inference stack loads. Demucs is selected
// by its YAML, which names the weights it needs; every other architecture is
// selected by the weights themselves. Deciding this from Architecture rather
// than from manifest order means a registry edit cannot silently load the wrong
// file.
func entryFile(m registry.Model) string {
	if m.Architecture == registry.ArchitectureDemucs {
		for _, f := range m.Files {
			if strings.HasSuffix(strings.ToLower(f.Dest), ".yaml") {
				return filepath.FromSlash(f.Dest)
			}
		}
	}
	for _, f := range m.Files {
		if !strings.HasSuffix(strings.ToLower(f.Dest), ".yaml") {
			return filepath.FromSlash(f.Dest)
		}
	}
	return filepath.FromSlash(m.Files[0].Dest)
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
// an already-16k-mono wav is a prerequisite the run does not have. When
// sepModelID is set the UVR5 runtime and weights are treated as prerequisites
// too, because separation is mandatory for real engines (ADR-037).
func Diagnose(workdir string, reg registry.Registry, backend registry.Backend, modelID string, needsDemux bool, sepModelID string) Missing {
	var m Missing

	if _, ok := FindRuntime(workdir, backend); !ok {
		// Naming an unpublished artifact sends the user to an install that must
		// refuse (sha256 tbd). Suggest the first preference that can actually be
		// installed; cuda12 is preferred on an NVIDIA host but has no published
		// build, so the hint would otherwise be a dead end.
		m.Runtime = suggestRuntime(reg, backend)
	}

	if modelID == "" {
		// No model chosen: name the cheapest usable default so the message is
		// actionable instead of "pick a model".
		m.Models = append(m.Models, "whisper-base")
	} else if _, ok := FindModel(workdir, reg, modelID); !ok {
		m.Models = append(m.Models, modelID)
	}

	if sepModelID != "" {
		if _, ok := FindSeparatorPython(workdir); !ok {
			m.SeparatorRuntime = true
		}
		if _, ok := FindModel(workdir, reg, sepModelID); !ok {
			m.SeparatorModels = append(m.SeparatorModels, sepModelID)
		}
	}

	if needsDemux {
		if _, ok := FindFFmpeg(workdir); !ok {
			m.FFmpeg = true
		}
	}
	if sepModelID != "" {
		// The separator shells out to ffmpeg for decoding and writes 44.1 kHz wav.
		if _, ok := FindFFmpeg(workdir); !ok {
			m.FFmpeg = true
		}
	}
	return m
}
