package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/engine"
	"github.com/hhx465453939/stemscribe/internal/pipeline"
	"github.com/hhx465453939/stemscribe/internal/registry"
	"github.com/hhx465453939/stemscribe/internal/resource"
	"github.com/hhx465453939/stemscribe/internal/store"
)

// backendPreference is the order to try once hardware is known.
func backendPreference(preferred registry.Backend) []registry.Backend {
	switch preferred {
	case registry.BackendCUDA12:
		return []registry.Backend{registry.BackendCUDA12, registry.BackendVulkan, registry.BackendCPU}
	case registry.BackendVulkan:
		return []registry.Backend{registry.BackendVulkan, registry.BackendCPU}
	default:
		return []registry.Backend{registry.BackendCPU}
	}
}

// backendFor picks the backend that will actually be used.
//
// Hardware capability alone is not enough: a machine with an NVIDIA GPU but only
// the cpu runtime installed must still run, otherwise "install the cpu runtime"
// is a dead end on exactly the machines most likely to try it. The installed set
// is therefore consulted, and any fallback is named in the reason so the UI can
// show why the GPU is idle.
func backendFor(locked, workdir string) (registry.Backend, string) {
	probe := resource.NewProbe()
	hw := resource.Hardware{}
	if total, err := probe.TotalMB(); err == nil && total > 0 {
		hw.HasNVIDIA = resource.LooksNVIDIA()
		hw.DriverMajor = resource.DriverMajor()
		hw.CUDA12Available = resource.CUDA12Present()
	}
	hw.VulkanAvailable = resource.VulkanPresent()

	sel := resource.SelectBackend(hw, resource.Backend(locked))

	var preferred registry.Backend
	switch string(sel.Backend) {
	case string(registry.BackendCUDA12):
		preferred = registry.BackendCUDA12
	case string(registry.BackendVulkan):
		preferred = registry.BackendVulkan
	default:
		preferred = registry.BackendCPU
	}

	for _, candidate := range backendPreference(preferred) {
		if _, ok := engine.FindRuntime(workdir, candidate); ok {
			if candidate == preferred {
				return candidate, sel.Reason
			}
			return candidate, fmt.Sprintf("%s preferred but not installed; using %s", preferred, candidate)
		}
	}
	return preferred, sel.Reason + "; no runtime installed yet"
}

// buildEngine assembles the pipeline engine.
//
// "synthetic" stays available on purpose: it is the test double that lets the
// pipeline be exercised without a 78 MB model download, and CHG-0001's smoke test
// depends on it. It is never the default.
func buildEngine(kind, workdir, modelID, locked string, durationMS int, demux bool, params engine.WhisperParams) (pipeline.Engine, string, error) {
	if strings.EqualFold(kind, "synthetic") {
		return newSyntheticEngine(durationMS), "synthetic (test double, does not read audio)", nil
	}

	reg, err := registry.Builtin()
	if err != nil {
		return nil, "", fmt.Errorf("load registry: %w", err)
	}

	backend, reason := backendFor(locked, workdir)
	missing := engine.Diagnose(workdir, reg, backend, modelID, demux)
	if !missing.Empty() {
		// Surface every gap at once; the caller prints this verbatim.
		return nil, "", missing.Err()
	}

	rt, ok := engine.FindRuntime(workdir, backend)
	if !ok {
		return nil, "", fmt.Errorf("runtime for backend %s vanished after diagnosis", backend)
	}
	modelPath, ok := engine.FindModel(workdir, reg, modelID)
	if !ok {
		return nil, "", fmt.Errorf("model %s vanished after diagnosis", modelID)
	}

	w := &engine.Whisper{
		Exe:       rt.Entry,
		ModelPath: modelPath,
		Backend:   string(backend),
		Version:   rt.Version,
		Params:    params,
	}
	desc := fmt.Sprintf("%s %s (%s)", "whisper.cpp", rt.Version, backend)
	if reason != "" {
		desc += " · " + reason
	}
	return w, desc, nil
}

// inputNeedsDemux reports whether the file must be converted first.
//
// A .wav is passed through untouched: whisper-cli's only requirement is 16 kHz
// mono 16-bit PCM, and refusing a ready-made wav just because ffmpeg is absent
// would make the tool unusable on machines that already have the right format.
func inputNeedsDemux(path string) bool {
	return !strings.EqualFold(filepath.Ext(path), ".wav")
}

// prepareAudio guarantees <bundle>/audio/16k.wav exists, which is the path the
// pipeline hands to the engine.
func prepareAudio(layout config.Layout, bundle *store.Bundle, input string, demux bool) error {
	dest, err := bundle.Path("audio/16k.wav")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	if demux {
		exe, ok := engine.FindFFmpeg(layout.Root)
		if !ok {
			return fmt.Errorf("input is %s and needs demuxing, but ffmpeg was not found", strings.ToLower(filepath.Ext(input)))
		}
		ff := &engine.FFmpeg{Exe: exe}
		return ff.ToWhisperWAV(context.Background(), input, dest)
	}

	// Copy rather than reference: the bundle must stay self-contained so it can be
	// moved or re-run after the original is gone.
	src, err := os.Open(input)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err = io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}
