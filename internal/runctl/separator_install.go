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
	"github.com/hhx465453939/stemscribe/internal/registry"
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

// PyTorch CUDA picks, verified by installing and running them on this host
// (2026-11-26): torch 2.14.1+cu126 drives CUDA 12.6 through the same 566-class
// driver as the ONNX runtime, and torchvision 0.29.1+cu126 matches it.
//
// 2.5.1+cu121 is not an option: PyPI's current torchvision 0.29.2 requires
// torch>=2.14.0, so downgrading torch breaks the dependency resolution of an
// otherwise healthy environment. Pinning the exact local version also keeps pip
// from preferring PyPI's newer CPU-only torch.
const (
	torchPackage       = "torch==2.14.1+cu126"
	torchvisionPackage = "torchvision==0.29.1+cu126"
)

// SeparatorInstallOptions selects runtime capabilities beyond the device flavour.
type SeparatorInstallOptions struct {
	// Torch installs the CUDA PyTorch build. It is 2.6 GB and only VR/Demucs/
	// MDX23C/Roformer models need it; an MDX-only user must not be forced to
	// download it (SPEC §6). Demucs without this profile runs on the CPU torch
	// wheel -- the exact silent downgrade this change exists to prevent.
	Torch bool
}

// torchIndexes is the ordered list of package indexes serving the cu126 wheels.
//
// Measured 2026-11-26 from this network: all three answer, SJTU answers the
// index page in 0.27 s and served the 2.6 GB wheel at ~7.7 MB/s, while the
// official index took 2.8 s for the same page. An explicit override wins so a
// mirrored deployment can point at its own artifact store.
func torchIndexes() []string {
	if v := strings.TrimSpace(os.Getenv("STEMSCRIBE_TORCH_INDEX")); v != "" {
		return []string{v}
	}
	return []string{
		"https://mirror.sjtu.edu.cn/pytorch-wheels/cu126/",
		"https://download.pytorch.org/whl/cu126",
		"https://mirrors.aliyun.com/pytorch-wheels/cu126/",
	}
}

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
	// TorchBackend is the device the installed PyTorch can really compute on:
	// "cuda", "cpu" or "absent". It is separate from Accel because the two stacks
	// fail independently -- an ONNX-only CUDA environment has a CPU torch wheel.
	TorchBackend string `json:"torch_backend,omitempty"`
	TorchReason  string `json:"torch_reason,omitempty"`
	TorchVersion string `json:"torch_version,omitempty"`
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
func InstallSeparatorRuntime(ctx context.Context, layout config.Layout, opts SeparatorInstallOptions, onProgress func(Progress)) (Installed, error) {
	if onProgress == nil {
		onProgress = func(Progress) {}
	}
	dir := filepath.Join(layout.Runtimes(), "separator")
	venv := filepath.Join(dir, "venv")
	python := venvPython(venv)
	desired := SeparatorDesiredAccel()
	desiredTorch := "cpu"
	if opts.Torch && desired == "cuda12" {
		desiredTorch = "cuda"
	}

	onProgress(Progress{
		Stage: "start", Kind: "runtime", ID: engine.SeparatorRuntimeID,
		SourceURL: "https://pypi.org/project/audio-separator/",
	})

	logf := func(format string, args ...any) {
		onProgress(Progress{Stage: "log", Note: fmt.Sprintf(format, args...)})
	}

	version, hasVenv := separatorInstalled(ctx, python)
	previous := engine.ReadSeparatorRuntime(layout.Root)
	wantDevice := previous.Accel == desired
	wantTorch := !opts.Torch || desiredTorch == "cpu" || previous.TorchBackend == desiredTorch
	if hasVenv && wantDevice && wantTorch {
		// Already installed in the flavour this machine wants. Refresh the record:
		// an environment provisioned before INSTALLED.json existed still gets one,
		// and "unknown" is a worse provenance answer than writing it now.
		if err := writeSeparatorRecord(dir, separatorRecord{
			verifiedFile: verifiedFile{
				ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
				SourceURL: "https://pypi.org/project/audio-separator/", InstalledAt: time.Now().UTC(),
			},
			Accel:        previous.Accel,
			Providers:    previous.Providers,
			AccelReason:  previous.Reason,
			TorchBackend: previous.TorchBackend,
			TorchReason:  previous.TorchReason,
			TorchVersion: previous.TorchVersion,
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

	// A healthy ONNX install that only lacks the CUDA torch profile is repaired in
	// place. Rebuilding the venv would re-download onnxruntime-gpu and the nvidia
	// wheels for no reason.
	torchOnly := hasVenv && wantDevice && opts.Torch && previous.TorchBackend != desiredTorch
	if torchOnly {
		logf("replacing torch with the CUDA build (%s)", torchPackage)
		if err := replaceTorch(ctx, python, useUV, onProgress, logf); err != nil {
			return Installed{}, err
		}
	} else {
		if hasVenv {
			logf("reinstalling separation runtime: recorded accel %q, this machine wants %q",
				previous.Accel, desired)
		}
	}

	if !torchOnly {
		if hasVenv {
			// Nothing to keep: the device flavour changes, so the whole environment
			// is rebuilt rather than partially repaired.
			_ = os.RemoveAll(venv)
		}
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

		// Install the CUDA torch build before audio-separator: pip then sees the
		// torch>=2.3 requirement already satisfied and never downloads the 700 MB
		// CPU wheel first (measured: a plain install pulls torch 2.15.0+cpu).
		if opts.Torch && desired == "cuda12" {
			if err := installTorchCUDA(ctx, python, useUV, onProgress, logf); err != nil {
				return Installed{}, err
			}
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
	}

	version, ok := separatorInstalled(ctx, python)
	if !ok {
		return Installed{}, fmt.Errorf("installed %s but the environment cannot import it; check the log above", separatorBasePackage)
	}

	// Ask onnxruntime which device it can really use. A package being present is
	// not proof: the CUDA provider is listed even when its DLLs cannot load, so
	// the probe creates a session and reports the providers it activated.
	probe := probeSeparatorSession(ctx, python)
	torchProbe := probeTorchDevice(ctx, python)
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

	// The torch probe is recorded even when the torch profile was not requested:
	// audio-separator always installs *a* torch (the CPU wheel), and "torch
	// present but CPU-only" is exactly the state a Demucs run must not discover
	// halfway through.
	torchBackend := torchProbe.Torch
	if torchBackend == "" {
		torchBackend = "absent"
	}
	torchReason := ""
	if opts.Torch && desiredTorch == "cuda" && torchBackend != "cuda" {
		torchReason = torchProbe.describeTorchFailure()
		logf("WARNING CUDA torch was requested but is not usable: %s", torchReason)
	}
	logf("separation runtime ready: accel=%s providers=%v torch=%s %s",
		accel, probe.Session, torchBackend, torchProbe.TorchVersion)

	record := separatorRecord{
		verifiedFile: verifiedFile{
			ID: engine.SeparatorRuntimeID, Kind: "runtime", Version: version,
			SourceURL: "https://pypi.org/project/audio-separator/", InstalledAt: time.Now().UTC(),
		},
		Accel:        accel,
		Providers:    probe.Session,
		AccelReason:  reason,
		TorchBackend: torchBackend,
		TorchReason:  torchReason,
		TorchVersion: torchProbe.TorchVersion,
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

// installTorchCUDA replaces the CPU PyTorch wheel with the CUDA 12 build.
//
// The index is tried in torchIndexes order and every failure is reported: the
// user sees which mirror was attempted and the package manager's last line. It
// never falls back to the CPU wheel -- that would leave Demucs computing on the
// CPU while the Models page says CUDA, which is the failure class this change
// exists to eliminate.
func installTorchCUDA(ctx context.Context, python string, useUV bool, onProgress func(Progress), logf func(string, ...any)) error {
	if err := uninstallTorch(ctx, python, useUV, onProgress); err != nil {
		return err
	}
	var failures []string
	for _, index := range torchIndexes() {
		logf("installing %s, %s from %s", torchPackage, torchvisionPackage, index)
		var tail string
		var err error
		if useUV {
			args := []string{"pip", "install", "--python", python,
				"--extra-index-url", index, torchPackage, torchvisionPackage}
			tail, err = runLoggedTail(ctx, onProgress, "uv", args...)
		} else {
			args := []string{"-m", "pip", "install",
				"--extra-index-url", index, torchPackage, torchvisionPackage}
			tail, err = runLoggedTail(ctx, onProgress, python, args...)
		}
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if tail == "" {
			tail = err.Error()
		}
		failures = append(failures, index+": "+tail)
		logf("WARNING %s failed (%s); trying the next index", index, tail)
	}
	return fmt.Errorf("could not install %s from any index:\n  %s",
		torchPackage, strings.Join(failures, "\n  "))
}

// uninstallTorch removes both PyTorch packages before a CUDA install. Without
// this pip can keep the CPU distribution and only add the CUDA one on top,
// leaving whichever DLL search order wins to decide the device.
func uninstallTorch(ctx context.Context, python string, useUV bool, onProgress func(Progress)) error {
	if useUV {
		return runLogged(ctx, onProgress, "uv", "pip", "uninstall", "--python", python,
			"torch", "torchvision")
	}
	return runLogged(ctx, onProgress, python, "-m", "pip", "uninstall", "-y",
		"torch", "torchvision")
}

// replaceTorch is the in-place repair for an environment whose ONNX side is
// healthy but whose torch is the CPU wheel. It does not touch onnxruntime or the
// nvidia wheels, so the 2.6 GB torch download is the only network cost.
func replaceTorch(ctx context.Context, python string, useUV bool, onProgress func(Progress), logf func(string, ...any)) error {
	return installTorchCUDA(ctx, python, useUV, onProgress, logf)
}

// separatorTorchRequired reports whether an installed separation model needs the
// CUDA torch build.
//
// Only models that are actually on disk count: a Demucs model sitting uninstalled
// in the catalog must not make every CUDA user download 2.6 GB. Installing the
// model is the point where the requirement becomes real (see ensureTorchRuntime).
func separatorTorchRequired(layout config.Layout) bool {
	reg, err := registry.Builtin()
	if err != nil {
		return false
	}
	for _, m := range reg.Models {
		if m.Engine != registry.EngineSeparator || !m.Architecture.UsesTorch() {
			continue
		}
		if _, ok := engine.FindModel(layout.Root, reg, m.ID); ok {
			return true
		}
	}
	return false
}

// ensureTorchRuntime upgrades the separation environment after a torch-based
// model is installed. It is silent on CPU-only machines (the CPU torch wheel is
// the truthful device there) and when the CUDA torch build is already active.
//
// A failure here is returned, not swallowed: the model is useless without it,
// and ADR-037 makes a half-provisioned separation an error the user must see.
func ensureTorchRuntime(ctx context.Context, layout config.Layout, onProgress func(Progress)) error {
	if _, ok := engine.FindSeparatorPython(layout.Root); !ok {
		// Onboarding will install the runtime later, and separatorTorchRequired will
		// then see this model and include torch in that install.
		return nil
	}
	if SeparatorDesiredAccel() != "cuda12" {
		return nil
	}
	info := engine.ReadSeparatorRuntime(layout.Root)
	if info.TorchBackend == "cuda" {
		return nil
	}
	if onProgress != nil {
		onProgress(Progress{Stage: "log", Note: "this model runs on PyTorch; adding the CUDA torch build to the separation runtime"})
	}
	_, err := InstallSeparatorRuntime(ctx, layout, SeparatorInstallOptions{Torch: true}, onProgress)
	return err
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
try:
    session = ort.InferenceSession(model.SerializeToString(), providers=["CUDAExecutionProvider"])
    print("SESSION:" + ",".join(session.get_providers()))
except Exception as e:
    print("SESSIONERROR:" + (str(e).splitlines() or ["?"])[-1][:200])
`

// separatorTorchProbeScript is a separate process on purpose.
//
// Creating the ONNX CUDA session loads onnxruntime's cuDNN into the process.
// Measured 2026-11-26: importing torch afterwards then failed with
// "[WinError 127] Error loading torch\lib\cudnn_cnn64_9.dll", even though the
// same torch computed on CUDA fine in a clean process. Two stacks, two probes:
// the recorded device must describe what each stack can do on its own.
const separatorTorchProbeScript = `
try:
    import torch
    print("TORCHVERSION:" + torch.__version__)
    cuda_ok = False
    if torch.cuda.is_available():
        try:
            cuda_ok = float(torch.zeros(1, device="cuda").sum().item()) == 0.0
        except Exception as e:
            print("TORCHERR:" + (str(e).splitlines() or ["?"])[-1][:200])
    if cuda_ok:
        print("TORCH:CUDA")
    elif float(torch.zeros(1).sum().item()) == 0.0:
        print("TORCH:CPU")
    else:
        print("TORCH:BROKEN")
except Exception as e:
    print("TORCH:ABSENT")
    print("TORCHERR:" + (str(e).splitlines() or ["?"])[-1][:200])
`

type separatorProbeResult struct {
	Available    []string
	Session      []string
	Err          error
	Output       string
	SessionErr   string
	Torch        string
	TorchVersion string
	TorchErr     string
}

func (p separatorProbeResult) describeFailure() string {
	if p.SessionErr != "" {
		return "CUDA probe failed: " + p.SessionErr
	}
	if p.Err == nil {
		return "onnxruntime did not activate CUDAExecutionProvider (session providers: " +
			strings.Join(p.Session, ",") + ")"
	}
	return "CUDA probe failed: " + p.lastLine()
}

func (p separatorProbeResult) describeTorchFailure() string {
	if p.TorchErr != "" {
		return "torch CUDA probe failed: " + p.TorchErr
	}
	switch p.Torch {
	case "cpu":
		return "torch is installed but CUDA is unavailable (CPU-only wheel or no visible driver)"
	case "broken":
		return "torch is installed but its CPU tensor operation failed"
	case "absent", "":
		return "torch is not installed in the separation runtime"
	}
	return "torch reported " + p.Torch
}

// lastLine is the final non-empty output line: Python tracebacks put the real
// error there and the frames above it are not actionable for a user.
func (p separatorProbeResult) lastLine() string {
	detail := strings.TrimSpace(p.Output)
	if i := strings.LastIndex(detail, "\n"); i >= 0 {
		detail = strings.TrimSpace(detail[i+1:])
	}
	if len(detail) > 300 {
		detail = detail[:300] + "..."
	}
	return detail
}

func probeSeparatorSession(ctx context.Context, python string) separatorProbeResult {
	probeCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, python, "-c", separatorProbeScript)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	out, err := cmd.CombinedOutput()
	return parseSeparatorProbe(string(out), err)
}

// parseSeparatorProbe turns the probe script's tagged lines into a result. It is
// separate from the process so the parsing is covered without a Python runtime.
func parseSeparatorProbe(output string, err error) separatorProbeResult {
	res := separatorProbeResult{Err: err, Output: strings.TrimSpace(output)}
	for _, line := range strings.Split(res.Output, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "AVAILABLE:"); ok {
			res.Available = splitList(v)
		}
		if v, ok := strings.CutPrefix(line, "SESSION:"); ok {
			res.Session = splitList(v)
		}
		for prefix, dst := range map[string]*string{
			"SESSIONERROR:": &res.SessionErr,
			"TORCH:":        &res.Torch,
			"TORCHVERSION:": &res.TorchVersion,
			"TORCHERR:":     &res.TorchErr,
		} {
			if v, ok := strings.CutPrefix(line, prefix); ok {
				*dst = strings.TrimSpace(v)
			}
		}
	}
	res.Torch = strings.ToLower(res.Torch)
	return res
}

// probeTorchDevice runs the PyTorch probe in its own process so ONNX's cuDNN
// cannot make a working torch look broken.
func probeTorchDevice(ctx context.Context, python string) separatorProbeResult {
	probeCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, python, "-c", separatorTorchProbeScript)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	out, err := cmd.CombinedOutput()
	return parseSeparatorProbe(string(out), err)
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
	_, err := runLoggedTail(ctx, onProgress, exe, args...)
	return err
}

// runLoggedTail is runLogged plus the last non-empty output line, so a failed
// package index attempt can be reported with its actual reason instead of
// "exit status 1".
func runLoggedTail(ctx context.Context, onProgress func(Progress), exe string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	last := ""
	w := &lineWriter{emit: func(line string) {
		last = line
		if onProgress != nil {
			onProgress(Progress{Stage: "log", Note: line})
		}
	}}
	cmd.Stdout = w
	cmd.Stderr = w
	err := cmd.Run()
	w.flush()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return last, ctxErr
		}
		return last, err
	}
	return last, nil
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
