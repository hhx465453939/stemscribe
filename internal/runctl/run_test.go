package runctl

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hhx465453939/stemscribe/internal/engine"
)

func TestIsAllocationFailure(t *testing.T) {
	yes := []string{
		"failed to allocate Vulkan buffer",
		"ggml_vulkan: ErrorOutOfDeviceMemory",
		"vk::Device::allocateMemory: ErrorOutOfDeviceMemory",
		"CUDA error: out of memory",
		"whisper_init: not enough memory",
	}
	for _, msg := range yes {
		if !isAllocationFailure(errors.New(msg)) {
			t.Errorf("expected allocation failure for %q", msg)
		}
	}
	no := []string{"exit status 10", "failed to open model file", "context canceled"}
	for _, msg := range no {
		if isAllocationFailure(errors.New(msg)) {
			t.Errorf("unexpected allocation failure for %q", msg)
		}
	}
	if isAllocationFailure(nil) {
		t.Error("nil error classified as allocation failure")
	}
}

func TestNextSmallerInstalledWalksTheLadder(t *testing.T) {
	wd := t.TempDir()
	mk := func(id, dest string) {
		dir := filepath.Join(wd, "models", "whisper", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, dest), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("whisper-medium", "ggml-medium.bin")
	mk("whisper-tiny", "ggml-tiny.bin")

	got, ok := nextSmallerInstalled(wd, "whisper-large-v3")
	if !ok || got != "whisper-medium" {
		t.Fatalf("next smaller = %q,%v want whisper-medium,true", got, ok)
	}
	got, ok = nextSmallerInstalled(wd, "whisper-medium")
	if !ok || got != "whisper-tiny" {
		t.Fatalf("next smaller after medium = %q,%v want whisper-tiny,true", got, ok)
	}
	if _, ok = nextSmallerInstalled(wd, "whisper-tiny"); ok {
		t.Fatal("tiny has no smaller installed model")
	}
	if _, ok = nextSmallerInstalled(wd, "unknown-model"); ok {
		t.Fatal("unknown model must not fall back to the largest one")
	}
}

// Provenance must not attribute the separator's stems to whisper.cpp: the two run
// as different processes with different versions. The separation device is
// recorded as reported, not hardcoded: a GPU run and a CPU fallback used to write
// the same "cpu" here, which is how a CPU-bound separation stayed invisible.
func TestBuildMetaKeepsEngineVersionsSeparate(t *testing.T) {
	meta := BuildMeta("v1", "t", "C:\\in.mp4", "done",
		"whisper.cpp", "1.9.5", "cuda12", "whisper-large-v3", "uvr-mdx-kim-vocal-2", "0.48.0", "cuda12",
		engine.Defaults())
	sep := meta.Provenance.Engines["separate"]
	if sep.Version != "0.48.0" {
		t.Fatalf("separate version = %q want 0.48.0", sep.Version)
	}
	if sep.ModelID != "uvr-mdx-kim-vocal-2" {
		t.Fatalf("separate model = %q", sep.ModelID)
	}
	if sep.Backend != "cuda12" {
		t.Fatalf("separate backend = %q want cuda12", sep.Backend)
	}
	tr := meta.Provenance.Engines["transcribe"]
	if tr.Version != "1.9.5" {
		t.Fatalf("transcribe version = %q", tr.Version)
	}
	if tr.Backend != "cuda12" {
		t.Fatalf("transcribe backend = %q want cuda12", tr.Backend)
	}
}

// A separation that fell back to the CPU must be recorded as CPU even though CUDA
// was requested; an unrun stage stays empty rather than claiming a device.
func TestBuildMetaRecordsSeparationFallback(t *testing.T) {
	meta := BuildMeta("v1", "t", "C:\\in.mp4", "done",
		"whisper.cpp", "1.9.5", "cuda12", "whisper-large-v3", "uvr-mdx-kim-vocal-2", "0.48.0", "cpu",
		engine.Defaults())
	if got := meta.Provenance.Engines["separate"].Backend; got != "cpu" {
		t.Fatalf("separate backend = %q want cpu", got)
	}
	meta = BuildMeta("v1", "t", "C:\\in.mp4", "failed",
		"whisper.cpp", "1.9.5", "cuda12", "whisper-large-v3", "uvr-mdx-kim-vocal-2", "0.48.0", "",
		engine.Defaults())
	if got := meta.Provenance.Engines["separate"].Backend; got != "" {
		t.Fatalf("unrun separation backend = %q want empty", got)
	}
}

func TestSeparationEnabledOnlyExemptsSynthetic(t *testing.T) {
	cases := map[string]bool{"": true, "auto": true, "auto ": true, "synthetic": false, "SYNTHETIC": false}
	for kind, want := range cases {
		if got := separationEnabled(kind); got != want {
			t.Errorf("separationEnabled(%q) = %v want %v", kind, got, want)
		}
	}
}

// The provenance backend must come from the separator's own report of the run,
// not from what was requested or from a default.
func TestSeparatorBackendReportsLastRun(t *testing.T) {
	comp := &engine.Composite{Separator: &engine.Separator{LastBackend: "cuda12"}}
	if got := separatorBackend(comp); got != "cuda12" {
		t.Fatalf("separatorBackend = %q want cuda12", got)
	}
	if got := separatorBackend(&engine.Composite{}); got != "" {
		t.Fatalf("separatorBackend without separator = %q want empty", got)
	}
	if got := separatorBackend(&engine.Whisper{}); got != "" {
		t.Fatalf("separatorBackend on a whisper-only engine = %q want empty", got)
	}
}
