package runctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/engine"
)

// separatorPackage is what the runtime install puts into the venv. The [cpu]
// extra is deliberate: it pins the CPU onnxruntime build, which is the only one
// that runs on every host without a CUDA toolkit.
const separatorPackage = "audio-separator[cpu]"

// InstallSeparatorRuntime builds <workdir>/runtimes/separator/venv with
// audio-separator installed.
//
// A Python environment cannot be shipped as a registry archive (it embeds absolute
// paths and a platform-specific interpreter), so this is the one runtime that is
// provisioned by running an installer rather than unpacking a verified zip. uv is
// preferred when present because it resolves and installs in seconds on this host;
// plain venv+pip is the fallback.
func InstallSeparatorRuntime(ctx context.Context, layout config.Layout, onProgress func(Progress)) (Installed, error) {
	if onProgress == nil {
		onProgress = func(Progress) {}
	}
	dir := filepath.Join(layout.Runtimes(), "separator")
	venv := filepath.Join(dir, "venv")
	python := venvPython(venv)

	onProgress(Progress{
		Stage: "start", Kind: "runtime", ID: engine.SeparatorRuntimeID,
		SourceURL: "https://pypi.org/project/audio-separator/",
	})

	if version, ok := separatorInstalled(ctx, python); ok {
		// An environment provisioned before INSTALLED.json existed still gets a
		// record: provenance reads the version from it, and "unknown" is a worse
		// answer than writing it now.
		record := verifiedFile{
			ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
			SourceURL: "https://pypi.org/project/audio-separator/", InstalledAt: time.Now().UTC(),
		}
		raw, _ := json.MarshalIndent(record, "", "  ")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "INSTALLED.json"), append(raw, '\n'), 0o644)
		}
		onProgress(Progress{Stage: "verify", Version: version})
		return Installed{
			ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
			SourceURL: "https://pypi.org/project/audio-separator/", DestDir: venv,
		}, nil
	}

	logf := func(format string, args ...any) {
		onProgress(Progress{Stage: "log", Note: fmt.Sprintf(format, args...)})
	}

	if _, err := exec.LookPath("uv"); err == nil {
		logf("creating environment with uv")
		if err := runLogged(ctx, onProgress, "uv", "venv", "--python", "3.12", venv); err != nil {
			return Installed{}, fmt.Errorf("create Python environment: %w", err)
		}
		logf("installing %s", separatorPackage)
		if err := runLogged(ctx, onProgress, "uv", "pip", "install", "--python", python, separatorPackage); err != nil {
			return Installed{}, fmt.Errorf("install %s: %w", separatorPackage, err)
		}
	} else {
		base, err := findBasePython()
		if err != nil {
			return Installed{}, err
		}
		logf("creating environment with %s", base)
		if err = runLogged(ctx, onProgress, base, "-m", "venv", venv); err != nil {
			return Installed{}, fmt.Errorf("create Python environment: %w", err)
		}
		logf("installing %s", separatorPackage)
		if err = runLogged(ctx, onProgress, python, "-m", "pip", "install", "--upgrade", "pip"); err != nil {
			return Installed{}, fmt.Errorf("upgrade pip: %w", err)
		}
		if err = runLogged(ctx, onProgress, python, "-m", "pip", "install", separatorPackage); err != nil {
			return Installed{}, fmt.Errorf("install %s: %w", separatorPackage, err)
		}
	}

	version, ok := separatorInstalled(ctx, python)
	if !ok {
		return Installed{}, fmt.Errorf("installed %s but the environment cannot import it; check the log above", separatorPackage)
	}
	record := verifiedFile{
		ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
		SourceURL: "https://pypi.org/project/audio-separator/", InstalledAt: time.Now().UTC(),
	}
	raw, _ := json.MarshalIndent(record, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "INSTALLED.json"), append(raw, '\n'), 0o644); err != nil {
		return Installed{}, fmt.Errorf("write install record: %w", err)
	}
	onProgress(Progress{Stage: "verify", Version: version})
	return Installed{
		ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
		SourceURL: "https://pypi.org/project/audio-separator/", DestDir: venv,
	}, nil
}

// venvPython is where a freshly created virtual environment puts its interpreter.
func venvPython(venv string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venv, "Scripts", "python.exe")
	}
	return filepath.Join(venv, "bin", "python")
}

// separatorInstalled reports whether python can import audio_separator, and its
// version. A missing interpreter is not an error here; it just means "install".
func separatorInstalled(ctx context.Context, python string) (string, bool) {
	if _, err := os.Stat(python); err != nil {
		return "", false
	}
	probe, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probe, python, "-c",
		"import audio_separator, importlib.metadata as m; print(m.version('audio-separator'))")
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", false
	}
	version := strings.TrimSpace(out.String())
	return version, version != ""
}

// findBasePython locates an interpreter for "python -m venv".
func findBasePython() (string, error) {
	if p := strings.TrimSpace(os.Getenv("STEMSCRIBE_PYTHON")); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	for _, name := range []string{"python", "python3"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no Python 3 interpreter found and uv is not installed; install Python 3.10+ or uv, then retry \"stemscribe runtime install %s\"", engine.SeparatorRuntimeID)
}

// runLogged runs a command and forwards each output line to the progress callback.
func runLogged(ctx context.Context, onProgress func(Progress), exe string, args ...string) error {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	w := &lineWriter{emit: func(line string) {
		onProgress(Progress{Stage: "log", Note: line})
	}}
	cmd.Stdout = w
	cmd.Stderr = w
	err := cmd.Run()
	w.flush()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	return nil
}

// lineWriter turns a byte stream into trimmed, non-empty lines.
type lineWriter struct {
	buf  bytes.Buffer
	emit func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			break
		}
		line := w.buf.Next(i + 1)
		if t := strings.TrimSpace(string(line)); t != "" {
			w.emit(t)
		}
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if t := strings.TrimSpace(w.buf.String()); t != "" {
		w.emit(t)
	}
	w.buf.Reset()
}
