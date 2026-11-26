package runctl

import (
	"sort"
	"strings"
	"testing"
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
