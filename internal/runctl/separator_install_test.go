package runctl

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/hhx465453939/stemscribe/internal/config"
)

// Exactly one ONNX Runtime flavour must be listed per variant. Installing both
// onnxruntime and onnxruntime-gpu leaves the provider list dependent on install
// order, which is how "CUDA installed" and "CPU actually used" coexisted.
func TestSeparatorPackagesPickOneORTFlavour(t *testing.T) {
	cuda := separatorPackages("cuda12")
	if containsString(cuda, "onnxruntime") {
		t.Fatal("cuda variant must not install the CPU onnxruntime package")
	}
	var gpu string
	for _, p := range cuda {
		if strings.HasPrefix(p, "onnxruntime-gpu") {
			gpu = p
		}
	}
	if gpu == "" {
		t.Fatal("cuda variant must install onnxruntime-gpu")
	}
	// The version is pinned because 1.24+ is built for CUDA 13 and silently fails
	// to load its provider on the CUDA 12 drivers this product targets.
	if !strings.Contains(gpu, "==") {
		t.Fatalf("onnxruntime-gpu must be pinned to a CUDA 12 build, got %q", gpu)
	}
	for _, dep := range []string{"nvidia-cudnn-cu12", "nvidia-cufft-cu12", "nvidia-cuda-runtime-cu12"} {
		if !containsString(cuda, dep) {
			t.Fatalf("cuda variant missing %s; onnxruntime cannot activate CUDA without it", dep)
		}
	}
	for _, variant := range [][]string{cuda, separatorPackages("cpu")} {
		// audio-separator imports audioread at module load but no longer declares it.
		if !containsString(variant, audioreadPackage) {
			t.Fatalf("variant %v missing %s", variant, audioreadPackage)
		}
	}

	cpu := separatorPackages("cpu")
	if !containsString(cpu, "onnxruntime") {
		t.Fatal("cpu variant must install onnxruntime")
	}
	if containsString(cpu, "onnxruntime-gpu") {
		t.Fatal("cpu variant must not install onnxruntime-gpu")
	}
	if !containsString(cpu, separatorBasePackage) {
		t.Fatal("both variants must install audio-separator")
	}

	// The CUDA list must be a strict superset of the base package, not a
	// replacement: audio-separator itself is never optional.
	all := append([]string{}, cuda...)
	sort.Strings(all)
	if all[0] != separatorBasePackage {
		t.Fatalf("cuda packages = %v", cuda)
	}
}

func TestSplitListDropsEmptyEntries(t *testing.T) {
	got := splitList(" CUDAExecutionProvider, , CPUExecutionProvider ")
	if len(got) != 2 || got[0] != "CUDAExecutionProvider" || got[1] != "CPUExecutionProvider" {
		t.Fatalf("splitList = %v", got)
	}
}

// The upgrade offer exists for exactly one state: installed on the wrong device.
func TestSeparatorNeedsUpgrade(t *testing.T) {
	cases := []struct {
		installed bool
		accel     string
		desired   string
		want      bool
	}{
		{true, "cpu", "cuda12", true},
		{true, "cuda12", "cpu", true},
		{true, "cuda12", "cuda12", false},
		{true, "cpu", "cpu", false},
		{false, "cpu", "cuda12", false}, // not installed: install, not upgrade
	}
	for _, c := range cases {
		if got := separatorNeedsUpgrade(c.installed, c.accel, c.desired); got != c.want {
			t.Errorf("separatorNeedsUpgrade(%v,%q,%q) = %v, want %v", c.installed, c.accel, c.desired, got, c.want)
		}
	}
}

// The CUDA torch build must never be silently swapped for the CPU wheel: the
// whole point of the profile is that a Demucs run either gets the GPU or fails
// loudly.
func TestTorchIndexesHonourOverride(t *testing.T) {
	t.Setenv("STEMSCRIBE_TORCH_INDEX", "https://example.org/wheels")
	got := torchIndexes()
	if len(got) != 1 || got[0] != "https://example.org/wheels" {
		t.Fatalf("torchIndexes = %v, want the explicit override only", got)
	}
	t.Setenv("STEMSCRIBE_TORCH_INDEX", "")
	if len(torchIndexes()) < 2 {
		t.Fatal("without an override there must be a fallback chain, not a single index")
	}
	if !strings.Contains(torchPackage, "+cu") || !strings.Contains(torchvisionPackage, "+cu") {
		t.Fatalf("torch pins must be CUDA wheels, got %q / %q", torchPackage, torchvisionPackage)
	}
}

func TestPyPIIndexesHonourOverride(t *testing.T) {
	t.Setenv("STEMSCRIBE_PYPI_INDEX", "https://example.org/simple/")
	got := pypiIndexes()
	if len(got) != 1 || got[0] != "https://example.org/simple/" {
		t.Fatalf("pypiIndexes = %v, want the explicit override only", got)
	}
	t.Setenv("STEMSCRIBE_PYPI_INDEX", "  ")
	got = pypiIndexes()
	if len(got) < 2 {
		t.Fatalf("pypiIndexes = %v, want a mirror plus the official fallback", got)
	}
	if got[len(got)-1] != "https://pypi.org/simple/" {
		t.Fatalf("last index = %q, want the official PyPI index so an offline mirror never blocks the install", got[len(got)-1])
	}
}

// TestMain doubles as a fake `uv`: TestInstallPyPIPackagesFallsBack stages a copy
// of this test binary on PATH under the name uv, and that copy takes the fakeUV
// branch instead of running tests.
func TestMain(m *testing.M) {
	if os.Getenv("STEMSCRIBE_FAKE_UV") == "1" {
		fakeUV()
	}
	os.Exit(m.Run())
}

// fakeUV answers like a package index that refuses one host and accepts another:
// the first index in the test fails with a message, the fallback exits 0.
func fakeUV() {
	args := strings.Join(os.Args[1:], " ")
	if strings.Contains(args, "bad.invalid") {
		fmt.Fprintln(os.Stderr, "error: could not reach bad.invalid")
		os.Exit(1)
	}
	fmt.Println("installed fake package")
	os.Exit(0)
}

// The retry is the safety property behind the mirror-first order: an index that
// answers and then fails must not leave the user without a runtime.
func TestInstallPyPIPackagesFallsBackToNextIndex(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locate the test binary: %v", err)
	}
	dir := t.TempDir()
	name := "uv"
	if runtime.GOOS == "windows" {
		name = "uv.exe"
	}
	fake := filepath.Join(dir, name)
	if err := copyExecutable(self, fake); err != nil {
		t.Fatalf("stage the fake uv: %v", err)
	}

	t.Setenv("STEMSCRIBE_FAKE_UV", "1")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	err = installFromIndexes(context.Background(), "python", true, []string{"pkg"},
		[]string{"https://bad.invalid/simple/", "https://good.invalid/simple/"}, nil, logf)
	if err != nil {
		t.Fatalf("installFromIndexes must retry the next index, got %v (log: %v)", err, lines)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "bad.invalid") || !strings.Contains(joined, "trying the next index") {
		t.Fatalf("the failed index must be reported, log was:\n%s", joined)
	}
	if !strings.Contains(joined, "good.invalid") {
		t.Fatalf("the fallback index was never attempted, log was:\n%s", joined)
	}
}

// copyExecutable copies src to dst with the executable bit set. Copying the test
// binary keeps the fake uv free of any build step or shell wrapper.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func TestDescribeTorchFailure(t *testing.T) {
	cases := []struct {
		probe separatorProbeResult
		want  string
	}{
		{separatorProbeResult{Torch: "cpu"}, "CPU-only"},
		{separatorProbeResult{Torch: "absent"}, "not installed"},
		{separatorProbeResult{Torch: "cpu", TorchErr: "DLL load failed"}, "DLL load failed"},
	}
	for _, c := range cases {
		if got := c.probe.describeTorchFailure(); !strings.Contains(got, c.want) {
			t.Errorf("describeTorchFailure(%+v) = %q, want it to mention %q", c.probe, got, c.want)
		}
	}
}

// A torch model on disk is what turns the 2.6 GB profile from optional into
// required; an uninstalled catalog entry must not.
func TestSeparatorTorchRequired(t *testing.T) {
	layout, err := config.NewLayout(t.TempDir())
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	if separatorTorchRequired(layout) {
		t.Fatal("nothing installed: torch profile must stay optional")
	}
	dir := filepath.Join(layout.Models(), "separator", "demucs-htdemucs-6s")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"htdemucs_6s.yaml", "5c90dfd2-34c22ccb.th"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !separatorTorchRequired(layout) {
		t.Fatal("an installed VR model needs the torch profile")
	}
}

// The probe output is the only evidence install records about the GPU. Parsing it
// wrongly would either claim CUDA without a session or hide a CPU-only torch.
func TestParseSeparatorProbe(t *testing.T) {
	out := `AVAILABLE:CUDAExecutionProvider,CPUExecutionProvider
SESSION:CUDAExecutionProvider,CPUExecutionProvider
TORCHVERSION:2.14.1+cu126
TORCH:CUDA`
	res := parseSeparatorProbe(out, nil)
	if !containsString(res.Session, "CUDAExecutionProvider") {
		t.Fatalf("session providers = %v", res.Session)
	}
	if res.Torch != "cuda" || res.TorchVersion != "2.14.1+cu126" {
		t.Fatalf("torch probe = %q version %q", res.Torch, res.TorchVersion)
	}

	// A CPU environment: the CUDA session raises, torch reports CPU, and both
	// reasons must survive parsing so the record can explain the downgrade.
	cpuOut := `AVAILABLE:CPUExecutionProvider
SESSIONERROR:CUDAExecutionProvider is not in available provider names
TORCHVERSION:2.15.0+cpu
TORCH:CPU`
	cpu := parseSeparatorProbe(cpuOut, nil)
	if len(cpu.Session) != 0 || cpu.SessionErr == "" {
		t.Fatalf("cpu probe session = %v err %q", cpu.Session, cpu.SessionErr)
	}
	if cpu.Torch != "cpu" {
		t.Fatalf("torch = %q", cpu.Torch)
	}
	if !strings.Contains(cpu.describeFailure(), "CUDAExecutionProvider is not in available provider names") {
		t.Fatalf("failure detail lost: %q", cpu.describeFailure())
	}
}
