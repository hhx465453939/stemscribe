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
	"github.com/hhx465453939/stemscribe/internal/resource"
)

// separatorBasePackage is what every install gets. The ONNX Runtime flavour is
// chosen separately: audio-separator's own [cpu] extra pins the CPU build, which
// is how separation computed on the CPU on a machine with a working CUDA GPU.
const separatorBasePackage = "audio-separator"

// audioreadPackage covers an import that audio-separator's metadata no longer
// declares: uvr_lib_v5/spec_utils.py imports audioread unconditionally, and a
// fresh venv without it fails at import time (measured 2026-11-26 on a clean
// Python 3.12 environment).
const audioreadPackage = "audioread"

// SeparatorDesiredAccel is the device a fresh install targets.
//
// CUDA only when an NVIDIA driver new enough for the CUDA 12 runtime is present;
// CPU otherwise. Exported so capabilities can describe an intended install
// without starting one.
func SeparatorDesiredAccel() string {
	if resource.LooksNVIDIA() && resource.CUDA12Present() {
		return "cuda12"
	}
	return "cpu"
}

// separatorPackages lists the pip requirements for an accel variant.
//
// onnxruntime-gpu resolves cublas, cufft and cudnn at load time. The nvidia-*
// wheels are the portable way to put those DLLs on Windows: requiring a system
// CUDA toolkit would make "install from the Models page" work only on dev boxes.
func separatorPackages(accel string) []string {
	if accel == "cuda12" {
		return []string{
			separatorBasePackage,
			// Pinned on purpose. onnxruntime-gpu 1.24+ is built against CUDA 13,
			// which needs a 580+ driver; the 566-class drivers most CUDA 12
			// machines run would list CUDAExecutionProvider and then fail to load
			// it (measured: provider DLL depends on cublasLt64_13.dll). 1.21.1 is
			// the last CUDA 12 build and runs on every driver that can run our
			// CUDA 12 whisper runtime.
			"onnxruntime-gpu==1.21.1",
			"nvidia-cublas-cu12",
			"nvidia-cudnn-cu12",
			"nvidia-cufft-cu12",
			"nvidia-curand-cu12",
			"nvidia-cuda-runtime-cu12",
			// audio-separator's uvr_lib_v5 imports audioread at module load, but its
			// package metadata stopped pulling it in (librosa 1.x dropped it). A fresh
			// venv without it dies with ModuleNotFoundError before the first session.
			audioreadPackage,
		}
	}
	return []string{separatorBasePackage, "onnxruntime", audioreadPackage}
}

// separatorRecord is INSTALLED.json for the Python environment. The accel fields
// did not exist before CHG-0017; their absence reads as "cpu", which is what
// those environments really were.
type separatorRecord struct {
	verifiedFile
	Accel       string   `json:"accel"`
	Providers   []string `json:"providers,omitempty"`
	AccelReason string   `json:"accel_reason,omitempty"`
}

// InstallSeparatorRuntime builds <workdir>/runtimes/separator/venv with
// audio-separator installed, on CUDA when this machine can run it.
//
// A Python environment cannot be shipped as a registry archive (it embeds absolute
// paths and a platform-specific interpreter), so this is the one runtime that is
// provisioned by running an installer rather than unpacking a verified zip. uv is
// preferred when present because it resolves and installs in seconds on this host;
// plain venv+pip is the fallback.
//
// The install is also the upgrade path: an environment recorded with a different
// accel is reinstalled with the requested ONNX Runtime flavour, because the CPU
// install and the GPU install differ only in packages, not in the venv.
func InstallSeparatorRuntime(ctx context.Context, layout config.Layout, onProgress func(Progress)) (Installed, error) {
	if onProgress == nil {
		onProgress = func(Progress) {}
	}
	dir := filepath.Join(layout.Runtimes(), "separator")
	venv := filepath.Join(dir, "venv")
	python := venvPython(venv)
	desired := SeparatorDesiredAccel()

	onProgress(Progress{
		Stage: "start", Kind: "runtime", ID: engine.SeparatorRuntimeID,
		SourceURL: "https://pypi.org/project/audio-separator/",
	})

	logf := func(format string, args ...any) {
		onProgress(Progress{Stage: "log", Note: fmt.Sprintf(format, args...)})
	}

	version, hasVenv := separatorInstalled(ctx, python)
	previous := engine.ReadSeparatorRuntime(layout.Root)
	if hasVenv && previous.Accel == desired {
		// Already installed in the flavour this machine wants. Refresh the record:
		// an environment provisioned before INSTALLED.json existed still gets one,
		// and "unknown" is a worse provenance answer than writing it now.
		if err := writeSeparatorRecord(dir, separatorRecord{
			verifiedFile: verifiedFile{
				ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
				SourceURL: "https://pypi.org/project/audio-separator/", InstalledAt: time.Now().UTC(),
			},
			Accel:       previous.Accel,
			Providers:   previous.Providers,
			AccelReason: previous.Reason,
		}); err != nil {
			return Installed{}, err
		}
		onProgress(Progress{Stage: "verify", Version: version})
		return Installed{
			ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
			SourceURL: "https://pypi.org/project/audio-separator/", DestDir: venv,
		}, nil
	}

	useUV := false
	if _, err := exec.LookPath("uv"); err == nil {
		useUV = true
	}

	if hasVenv {
		logf("reinstalling separation runtime: recorded accel %q, this machine wants %q",
			previous.Accel, desired)
	} else {
		if useUV {
			logf("creating environment with uv")
			if err := runLogged(ctx, onProgress, "uv", "venv", "--python", "3.12", venv); err != nil {
				return Installed{}, fmt.Errorf("create Python environment: %w", err)
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
			if err = runLogged(ctx, onProgress, python, "-m", "pip", "install", "--upgrade", "pip"); err != nil {
				return Installed{}, fmt.Errorf("upgrade pip: %w", err)
			}
		}
	}

	// Remove the other ONNX Runtime flavour first. Both distributions own the same
	// onnxruntime/ files, so leaving both installed makes the provider list depend
	// on install order -- the exact class of silent behaviour this change removes.
	conflicting := "onnxruntime-gpu"
	if desired == "cuda12" {
		conflicting = "onnxruntime"
	}
	if useUV {
		_ = runLogged(ctx, onProgress, "uv", "pip", "uninstall", "--python", python, conflicting)
	} else {
		_ = runLogged(ctx, onProgress, python, "-m", "pip", "uninstall", "-y", conflicting)
	}

	packages := separatorPackages(desired)
	logf("installing %s", strings.Join(packages, " "))
	if useUV {
		args := append([]string{"pip", "install", "--python", python}, packages...)
		if err := runLogged(ctx, onProgress, "uv", args...); err != nil {
			return Installed{}, fmt.Errorf("install %s: %w", strings.Join(packages, " "), err)
		}
	} else {
		args := append([]string{"-m", "pip", "install"}, packages...)
		if err := runLogged(ctx, onProgress, python, args...); err != nil {
			return Installed{}, fmt.Errorf("install %s: %w", strings.Join(packages, " "), err)
		}
	}

	version, ok := separatorInstalled(ctx, python)
	if !ok {
		return Installed{}, fmt.Errorf("installed %s but the environment cannot import it; check the log above", separatorBasePackage)
	}

	// Ask onnxruntime which device it can really use. A package being present is
	// not proof: the CUDA provider is listed even when its DLLs cannot load, so
	// the probe creates a session and reports the providers it activated.
	probe := probeSeparatorSession(ctx, python)
	accel := "cpu"
	reason := ""
	if desired == "cuda12" {
		if probe.Err == nil && containsString(probe.Session, "CUDAExecutionProvider") {
			accel = "cuda12"
		} else {
			reason = probe.describeFailure()
			logf("WARNING CUDA was requested but onnxruntime cannot activate it: %s", reason)
		}
	}
	logf("separation runtime ready: accel=%s providers=%v", accel, probe.Session)

	record := separatorRecord{
		verifiedFile: verifiedFile{
			ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
			SourceURL: "https://pypi.org/project/audio-separator/", InstalledAt: time.Now().UTC(),
		},
		Accel:       accel,
		Providers:   probe.Session,
		AccelReason: reason,
	}
	if err := writeSeparatorRecord(dir, record); err != nil {
		return Installed{}, err
	}
	onProgress(Progress{Stage: "verify", Version: version, Note: "accel=" + accel})
	return Installed{
		ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
		SourceURL: "https://pypi.org/project/audio-separator/", DestDir: venv,
	}, nil
}

func writeSeparatorRecord(dir string, record separatorRecord) error {
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install record: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "INSTALLED.json"), append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write install record: %w", err)
	}
	return nil
}

// separatorProbeScript reports the providers onnxruntime has, and the ones a real
// session activated. The PATH fix mirrors the helper: the nvidia-* wheels put
// their DLLs in site-packages/nvidia/*/bin, which is not on PATH by default.
const separatorProbeScript = `
import glob, os, sysconfig
dirs = []
for key in ("purelib", "platlib"):
    root = sysconfig.get_paths().get(key)
    if root:
        dirs.extend(glob.glob(os.path.join(root, "nvidia", "*", "bin")))
os.environ["PATH"] = os.pathsep.join(dirs + [os.environ.get("PATH", "")])
import onnxruntime as ort
print("AVAILABLE:" + ",".join(ort.get_available_providers()))
import onnx
from onnx import TensorProto, helper
node = helper.make_node("Identity", ["x"], ["y"])
graph = helper.make_graph([node], "probe",
    [helper.make_tensor_value_info("x", TensorProto.FLOAT, [1])],
    [helper.make_tensor_value_info("y", TensorProto.FLOAT, [1])])
model = helper.make_model(graph, opset_imports=[helper.make_opsetid("", 13)], ir_version=9)
session = ort.InferenceSession(model.SerializeToString(), providers=["CUDAExecutionProvider"])
print("SESSION:" + ",".join(session.get_providers()))
`

type separatorProbeResult struct {
	Available []string
	Session   []string
	Err       error
	Output    string
}

func (p separatorProbeResult) describeFailure() string {
	if p.Err == nil {
		return "onnxruntime did not activate CUDAExecutionProvider (session providers: " +
			strings.Join(p.Session, ",") + ")"
	}
	detail := strings.TrimSpace(p.Output)
	if i := strings.LastIndex(detail, "\n"); i >= 0 {
		detail = strings.TrimSpace(detail[i+1:])
	}
	if len(detail) > 300 {
		detail = detail[:300] + "..."
	}
	return "CUDA probe failed: " + detail
}

func probeSeparatorSession(ctx context.Context, python string) separatorProbeResult {
	probeCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, python, "-c", separatorProbeScript)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	out, err := cmd.CombinedOutput()
	res := separatorProbeResult{Err: err, Output: strings.TrimSpace(string(out))}
	for _, line := range strings.Split(res.Output, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "AVAILABLE:"); ok {
			res.Available = splitList(v)
		}
		if v, ok := strings.CutPrefix(line, "SESSION:"); ok {
			res.Session = splitList(v)
		}
	}
	return res
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
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

// venvPython is where a freshly created virtual environment puts its interpreter.
func venvPython(venv string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venv, "Scripts", "python.exe")
	}
	return filepath.Join(venv, "bin", "python")
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
