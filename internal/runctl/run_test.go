package runctl

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hhx465453939/stemscribe/internal/engine"
	"github.com/hhx465453939/stemscribe/internal/registry"
	"github.com/hhx465453939/stemscribe/internal/store"
)

// The retention set is decided before the run starts: a set the model cannot
// produce (or one without vocals) must be refused at the door, not after a
// multi-minute separation.
func TestNormalizeStems(t *testing.T) {
	m := registry.Model{ID: "demucs-x", Stems: []string{"vocals", "drums", "bass", "other"}}

	got, err := normalizeStems([]string{" bass ", "vocals", "bass"}, m)
	if err != nil {
		t.Fatalf("valid set rejected: %v", err)
	}
	if strings.Join(got, ",") != "bass,vocals" {
		t.Errorf("normalizeStems = %v, want sorted and de-duplicated", got)
	}

	if _, err := normalizeStems([]string{"drums"}, m); err == nil || !strings.Contains(err.Error(), "vocals") {
		t.Errorf("a set without vocals must be refused, got %v", err)
	}

	_, err = normalizeStems([]string{"vocals", "guitar"}, m)
	if err == nil || !strings.Contains(err.Error(), "guitar") || !strings.Contains(err.Error(), "vocals") {
		t.Errorf("unknown stem must be refused with the available list, got %v", err)
	}
}

// Unchecking a stem and resuming has to re-run separation; otherwise the
// checkpoint keeps claiming the old output is valid and nothing happens.
func TestSeparationHashTracksRetainedStems(t *testing.T) {
	all := separationHash("m", SeparateTuning{}, nil)
	vocals := separationHash("m", SeparateTuning{}, []string{"vocals"})
	two := separationHash("m", SeparateTuning{}, []string{"vocals", "drums"})
	if all == vocals || vocals == two || all == two {
		t.Fatalf("hash ignored the retained set: all=%s vocals=%s two=%s", all, vocals, two)
	}
	if two != separationHash("m", SeparateTuning{}, []string{"drums", "vocals"}) {
		t.Error("hash must not depend on the order the caller listed the stems")
	}
}

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

// A bundle reused across model changes must not keep stems from the previous
// model: the UI says unselected stems are deleted, and switching separators is
// the ordinary way to end up with an orphan such as UVR5's instrumental.wav.
func TestDropStaleAudioRemovesPreviousModelStems(t *testing.T) {
	root := t.TempDir()
	bundle, err := store.New(root)
	if err != nil {
		t.Fatalf("new bundle: %v", err)
	}
	audio := filepath.Join(root, "audio")
	if err := os.MkdirAll(audio, 0o755); err != nil {
		t.Fatalf("mkdir audio: %v", err)
	}
	names := []string{"full.wav", "16k.wav", "vocals16k.wav", "vocals.wav", "instrumental.wav"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(audio, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	removed, err := dropStaleAudio(bundle, []string{"vocals"}, "")
	if err != nil {
		t.Fatalf("dropStaleAudio: %v", err)
	}
	if strings.Join(removed, ",") != "instrumental" {
		t.Fatalf("removed = %v, want [instrumental]", removed)
	}
	for _, keep := range []string{"full.wav", "16k.wav", "vocals16k.wav", "vocals.wav"} {
		if _, statErr := os.Stat(filepath.Join(audio, keep)); statErr != nil {
			t.Errorf("%s must survive the sweep: %v", keep, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(audio, "instrumental.wav")); !os.IsNotExist(statErr) {
		t.Errorf("instrumental.wav should be gone, stat = %v", statErr)
	}

	// An unrecognised model with an empty keep set says nothing about which
	// stems are live, so the sweep must refuse to act instead of deleting all.
	if err := os.WriteFile(filepath.Join(audio, "instrumental.wav"), []byte("x"), 0o644); err != nil {
		t.Fatalf("restore instrumental: %v", err)
	}
	removed, err = dropStaleAudio(bundle, nil, "no-such-model")
	if err != nil {
		t.Fatalf("unknown model sweep: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("unknown model removed %v, want nothing", removed)
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
