package runctl

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

// BackendPreference is the order to try once hardware is known.
func BackendPreference(preferred registry.Backend) []registry.Backend {
	switch preferred {
	case registry.BackendCUDA12:
		return []registry.Backend{registry.BackendCUDA12, registry.BackendVulkan, registry.BackendCPU}
	case registry.BackendVulkan:
		return []registry.Backend{registry.BackendVulkan, registry.BackendCPU}
	default:
		return []registry.Backend{registry.BackendCPU}
	}
}

// BackendFor picks the backend that will actually be used.
//
// Hardware capability alone is not enough: a machine with an NVIDIA GPU but only
// the cpu runtime installed must still run. The installed set is therefore
// consulted, and any fallback is named in the reason so the UI can show why the
// GPU is idle (ADR-004: no silent downgrade).
func BackendFor(locked, workdir string) (registry.Backend, string) {
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

	for _, candidate := range BackendPreference(preferred) {
		if _, ok := engine.FindRuntime(workdir, candidate); ok {
			if candidate == preferred {
				return candidate, sel.Reason
			}
			return candidate, fmt.Sprintf("%s preferred but not installed; using %s", preferred, candidate)
		}
	}
	return preferred, sel.Reason + "; no runtime installed yet"
}

// BuildEngine assembles the pipeline engine.
//
// "synthetic" stays available on purpose: it is the test double that exercises the
// pipeline without a model download, and the smoke test depends on it. It is never
// the default.
func BuildEngine(kind, workdir, modelID, locked string, durationMS int, demux bool, params engine.WhisperParams) (pipeline.Engine, string, error) {
	if strings.EqualFold(kind, "synthetic") {
		return NewSyntheticEngine(durationMS), "synthetic (test double, does not read audio)", nil
	}

	reg, err := registry.Builtin()
	if err != nil {
		return nil, "", fmt.Errorf("load registry: %w", err)
	}

	backend, reason := BackendFor(locked, workdir)
	missing := engine.Diagnose(workdir, reg, backend, modelID, demux)
	if !missing.Empty() {
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

	// Validate before spending a second on audio. Skipping these guards let a
	// VAD-without-model request reach whisper-cli and come back as an opaque exit 10.
	if regModel, found := reg.FindModel(modelID); found {
		if err := params.Validate(regModel); err != nil {
			return nil, "", err
		}
	}

	w := &engine.Whisper{
		Exe:       rt.Entry,
		ModelPath: modelPath,
		Backend:   string(backend),
		Version:   rt.Version,
		Params:    params,
	}
	// Persist the engine's own output: a bare exit code is undiagnosable, and a bug
	// report needs something to attach. Best effort — never blocks a run.
	logPath := filepath.Join(workdir, "logs", "engine.log")
	if mkErr := os.MkdirAll(filepath.Dir(logPath), 0o755); mkErr == nil {
		if f, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); logErr == nil {
			w.Log = f
		}
	}
	desc := fmt.Sprintf("whisper.cpp %s (%s)", rt.Version, backend)
	if reason != "" {
		desc += " · " + reason
	}
	return w, desc, nil
}

// PrepareAudio guarantees <bundle>/audio/16k.wav exists, which is the path the
// pipeline hands to the engine.
//
// reuse must be true when the checkpoint already validated this file: rewriting it
// refreshes the mtime, and demux records (size, mod_time) as its fingerprint, so
// the next run would report "16k.wav modified" forever.
func PrepareAudio(ctx context.Context, layout config.Layout, bundle *store.Bundle, input string, demux, reuse bool) error {
	dest, err := bundle.Path("audio/16k.wav")
	if err != nil {
		return err
	}
	if reuse {
		if st, statErr := os.Stat(dest); statErr == nil && st.Size() > 0 {
			return nil
		}
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
		return ff.ToWhisperWAV(ctx, input, dest)
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

// BuildMeta assembles the bundle metadata, including a truthful provenance record.
func BuildMeta(videoID, title, inputPath, status, engName, engVersion string, params engine.WhisperParams) store.Meta {
	if title == "" {
		title = filepath.Base(inputPath)
	}
	hash := params.HashInput()
	if hash == "" {
		hash = "engine-defaults"
	}
	return store.Meta{
		VideoID: videoID,
		Title:   title,
		Status:  status,
		Source:  store.Source{Kind: "file", Origin: "local"},
		Stages:  map[string]store.StageRun{},
		Provenance: store.Provenance{
			// A real whisper.cpp run used to record "synthetic" here. Provenance that
			// lies is worse than none, because it is trusted.
			Engines: map[string]store.EngineInfo{
				"transcribe": {Name: engName, Version: engVersion},
			},
			ParamsSnapshotHash: store.HashParams(hash),
			ParamsSnapshot:     params.Snapshot(),
		},
	}
}

// WriteArtifacts writes the human-facing exports next to the transcript.
func WriteArtifacts(b *store.Bundle, title string, segs []pipeline.Segment) error {
	if title == "" {
		title = b.Dir()
	}
	files := map[string]string{
		"text/transcript.srt": pipeline.ToSRT(segs),
		"text/transcript.vtt": pipeline.ToVTT(segs),
		"text/transcript.md":  pipeline.ToMarkdown(title, segs),
	}
	for rel, content := range files {
		full, err := b.Path(rel)
		if err != nil {
			return err
		}
		if err = os.WriteFile(full, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}
