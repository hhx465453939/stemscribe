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
// as different processes with different versions.
func TestBuildMetaKeepsEngineVersionsSeparate(t *testing.T) {
	meta := BuildMeta("v1", "t", "C:\\in.mp4", "done",
		"whisper.cpp", "1.9.5", "cuda12", "whisper-large-v3", "uvr-mdx-kim-vocal-2", "0.48.0",
		engine.Defaults())
	sep := meta.Provenance.Engines["separate"]
	if sep.Version != "0.48.0" {
		t.Fatalf("separate version = %q want 0.48.0", sep.Version)
	}
	if sep.ModelID != "uvr-mdx-kim-vocal-2" {
		t.Fatalf("separate model = %q", sep.ModelID)
	}
	if sep.Backend != "cpu" {
		t.Fatalf("separate backend = %q want cpu (onnxruntime)", sep.Backend)
	}
	tr := meta.Provenance.Engines["transcribe"]
	if tr.Version != "1.9.5" {
		t.Fatalf("transcribe version = %q", tr.Version)
	}
	if tr.Backend != "cuda12" {
		t.Fatalf("transcribe backend = %q want cuda12", tr.Backend)
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
