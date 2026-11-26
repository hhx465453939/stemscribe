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
//
// sepModelID is the UVR5 model. When it is set the returned engine also separates,
// because separation is a mandatory stage of the real path (ADR-037); the only
// caller that passes "" is the synthetic test double.
func BuildEngine(kind, workdir, modelID, sepModelID, locked string, durationMS int, demux bool, params engine.WhisperParams) (pipeline.Engine, string, error) {
	if strings.EqualFold(kind, "synthetic") {
		return NewSyntheticEngine(durationMS), "synthetic (test double, does not read audio)", nil
	}

	reg, err := registry.Builtin()
	if err != nil {
		return nil, "", fmt.Errorf("load registry: %w", err)
	}

	backend, reason := BackendFor(locked, workdir)
	missing := engine.Diagnose(workdir, reg, backend, modelID, demux, sepModelID)
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

	var eng pipeline.Engine = w
	if sepModelID != "" {
		sep, sepDesc, sepErr := buildSeparator(workdir, reg, sepModelID)
		if sepErr != nil {
			return nil, "", sepErr
		}
		sep.Log = w.Log
		eng = &engine.Composite{Whisper: w, Separator: sep}
		desc += " · " + sepDesc
	}
	return eng, desc, nil
}

// buildSeparator resolves the UVR5 Python environment, the pinned metadata and the
// weights, and returns the adapter. Any gap is a hard error carrying the install
// command; separation is never skipped (AC-02).
func buildSeparator(workdir string, reg registry.Registry, sepModelID string) (*engine.Separator, string, error) {
	python, ok := engine.FindSeparatorPython(workdir)
	if !ok {
		return nil, "", fmt.Errorf("vocal separation runtime is missing: run \"stemscribe runtime install %s\"", engine.SeparatorRuntimeID)
	}
	helper, metaDir, err := engine.EnsureSeparatorAssets(workdir)
	if err != nil {
		return nil, "", err
	}
	modelPath, ok := engine.FindModel(workdir, reg, sepModelID)
	if !ok {
		return nil, "", fmt.Errorf("separation model %s is missing: run \"stemscribe model install %s\"", sepModelID, sepModelID)
	}
	ffmpeg, ok := engine.FindFFmpeg(workdir)
	if !ok {
		return nil, "", fmt.Errorf("vocal separation needs ffmpeg; put ffmpeg.exe under <workdir>/runtimes/ffmpeg/<ver>/ or add it to PATH")
	}
	m, _ := reg.FindModel(sepModelID)
	display := m.DisplayName
	if display == "" {
		display = sepModelID
	}
	return &engine.Separator{
		Python:    python,
		Helper:    helper,
		MetaDir:   metaDir,
		ModelDir:  filepath.Dir(modelPath),
		ModelFile: filepath.Base(modelPath),
		FFmpegExe: ffmpeg,
		Version:   engine.SeparatorVersion(workdir),
	}, display, nil
}

// PrepareAudio guarantees the two audio files the real path needs:
//
//	audio/full.wav  44.1 kHz stereo, the separator's input
//	audio/16k.wav   16 kHz mono, whisper's input when a run has no separation
//
// reuse must be true when the checkpoint already validated these files: rewriting
// them refreshes the mtime, and demux records (size, mod_time) as its fingerprint,
// so the next run would report "16k.wav modified" forever.
func PrepareAudio(ctx context.Context, layout config.Layout, bundle *store.Bundle, input string, demux, reuse bool) error {
	full, err := bundle.Path("audio/full.wav")
	if err != nil {
		return err
	}
	dest, err := bundle.Path("audio/16k.wav")
	if err != nil {
		return err
	}
	if reuse && nonEmpty(full) && nonEmpty(dest) {
		return nil
	}
	if err = os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	if exe, ok := engine.FindFFmpeg(layout.Root); ok {
		ff := &engine.FFmpeg{Exe: exe}
		return ff.ToWAVPair(ctx, input, full, dest)
	}
	if demux {
		return fmt.Errorf("input is %s and needs demuxing, but ffmpeg was not found", strings.ToLower(filepath.Ext(input)))
	}

	// No ffmpeg and an already-WAV source: the bundle stays self-contained by
	// copying. The separator still refuses to run without ffmpeg, because it needs
	// 44.1 kHz stereo rather than whatever the source happened to be.
	for _, dst := range []string{full, dest} {
		if err = copyFile(input, dst); err != nil {
			return err
		}
	}
	return nil
}

func nonEmpty(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// BuildMeta assembles the bundle metadata, including a truthful provenance record.
// modelID and sepModelID are recorded because a transcript without them cannot be
// reproduced or compared (AC-01/AC-03).
func BuildMeta(videoID, title, inputPath, status, engName, engVersion, modelID, sepModelID, sepVersion string, params engine.WhisperParams) store.Meta {
	if title == "" {
		title = filepath.Base(inputPath)
	}
	hash := params.HashInput()
	if hash == "" {
		hash = "engine-defaults"
	}
	engines := map[string]store.EngineInfo{
		"transcribe": {Name: engName, Version: engVersion, ModelID: modelID},
	}
	if sepModelID != "" {
		// The separator runs in its own Python environment: recording the whisper
		// version here would be a provenance lie.
		engines["separate"] = store.EngineInfo{Name: "uvr5", Version: sepVersion, ModelID: sepModelID}
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
			Engines:            engines,
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
		"text/transcript.txt": pipeline.ToText(segs),
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
