// Package runctl is the single entry point for running the pipeline.
//
// The orchestration used to live in cmd/stemscribe. That worked for the CLI and
// made a working desktop client impossible: cmd/stemscribe-gui is a different main
// package and cannot import another main package, so the window had no way to start
// a run -- which is why clicking Start did nothing at all. Moving it here gives the
// CLI, the browser shell and the native window one path instead of three (ADR-024).
package runctl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/engine"
	"github.com/hhx465453939/stemscribe/internal/pipeline"
	"github.com/hhx465453939/stemscribe/internal/registry"
	"github.com/hhx465453939/stemscribe/internal/store"
)

// Version is set by the entrypoint so provenance records the real build stamp.
var Version = "dev"

// Options is one run request. Every field is explicit: a caller that forgets a
// knob gets the engine default, never a silent zero value.
type Options struct {
	Input      string
	Title      string
	Workdir    string
	ModelID    string
	Engine     string // auto (real whisper.cpp) | synthetic (test double)
	DurationMS int
	Params     engine.WhisperParams
	Separate   bool
	// Emit receives pipeline events. All progress is derived from these; nothing is
	// invented by a timer (ADR-023).
	Emit func(pipeline.Event)
}

// Outcome reports what a finished run produced.
type Outcome struct {
	VideoID   string
	BundleDir string
	Segments  int
	Completed []store.Stage
}

// TotalStages is the progress denominator: the stages this run will actually
// report. Polish is never part of the local path, and separation only counts when
// it is enabled -- otherwise the bar could never reach 100%.
func TotalStages(separate bool) int {
	n := len(store.StageOrder) - 1
	if !separate {
		n--
	}
	if n < 1 {
		n = 1
	}
	return n
}

// Run executes one bundle end to end. It returns an error only for fatal
// conditions; a stage failure is reported through the error it carries.
func Run(ctx context.Context, o Options) (Outcome, error) {
	var out Outcome
	if o.Input == "" {
		return out, fmt.Errorf("input is required")
	}
	absInput, err := filepath.Abs(o.Input)
	if err != nil {
		return out, fmt.Errorf("resolve input: %w", err)
	}
	info, err := os.Stat(absInput)
	if err != nil {
		return out, fmt.Errorf("input not readable: %w", err)
	}
	if info.IsDir() {
		return out, fmt.Errorf("input must be a file, not a directory")
	}

	wd := o.Workdir
	if wd == "" {
		if wd, err = config.DefaultWorkdir(); err != nil {
			return out, fmt.Errorf("resolve default workdir: %w", err)
		}
	}
	settings, layout, err := LoadSettings(wd)
	if err != nil {
		return out, err
	}

	kind := o.Engine
	if kind == "" {
		kind = "auto"
	}
	if kind != "auto" && kind != "synthetic" {
		return out, fmt.Errorf("unknown engine %q (use auto or synthetic)", kind)
	}

	videoID, err := VideoID(absInput)
	if err != nil {
		return out, fmt.Errorf("derive video id: %w", err)
	}
	out.VideoID = videoID
	bundle, err := store.New(filepath.Join(layout.Libraries(), videoID))
	if err != nil {
		return out, fmt.Errorf("open bundle: %w", err)
	}
	if err = bundle.Ensure(); err != nil {
		return out, fmt.Errorf("prepare bundle: %w", err)
	}
	out.BundleDir = bundle.Dir()

	emit := o.Emit
	if emit == nil {
		emit = func(pipeline.Event) {}
	}

	// The run log is part of the bundle's provenance, not a CLI nicety, so it is
	// written here: every entrypoint gets an auditable record for free.
	runID := time.Now().UTC().Format("20060102T150405Z")
	runLog, logErr := os.Create(filepath.Join(bundle.Dir(), "runs", runID+".jsonl"))
	if logErr == nil {
		defer func() { _ = runLog.Close() }()
	}
	emitLogged := func(ev pipeline.Event) {
		if ev.TS == "" {
			ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if runLog != nil {
			if raw, mErr := json.Marshal(ev); mErr == nil {
				_, _ = runLog.Write(append(raw, '\n'))
			}
		}
		emit(ev)
	}

	params := o.Params
	demux := InputNeedsDemux(absInput)

	// Separation is opt-in twice over: the user must ask for it and settings must
	// allow it. Either one saying no is enough, and neither is inferred.
	separate := o.Separate && settings.Stages.Separate
	disabled := map[store.Stage]bool{
		store.StageSeparate: !separate,
		store.StagePolish:   true,
	}

	modelID := o.ModelID
	if modelID == "" {
		modelID = settings.Stages.Whisper
	}
	// Settings name a preferred model that may never have been downloaded. Failing the
	// very first click on "whisper-medium is missing" is technically honest but breaks
	// the promise that a new user can press Start and see it work. Fall back to the
	// best *installed* model and say so out loud (ADR-004: no silent downgrade).
	if reg, regErr := registry.Builtin(); regErr == nil {
		if id, note := resolveModel(layout.Root, reg, modelID); id != modelID {
			emitLogged(pipeline.Event{Kind: "stage.warn", Code: "MODEL_SUBSTITUTED", Note: note})
			modelID = id
		}
	}
	engName, engVersion := "synthetic", Version
	eng, engDesc, engErr := BuildEngine(kind, layout.Root, modelID,
		string(settings.Runtime.LockedBackend), o.DurationMS, demux, params)
	if engErr != nil {
		return out, engErr
	}
	emitLogged(pipeline.Event{Kind: "engine", Note: engDesc})
	if w, ok := eng.(*engine.Whisper); ok {
		engName, engVersion = "whisper.cpp", w.Version
	}

	// Separation moved to CHG-0003 (ADR-014). Saying so once and continuing keeps an
	// unavailable optional stage from failing an otherwise fine transcription.
	if separate {
		if _, isSynth := eng.(*SyntheticEngine); !isSynth {
			emitLogged(pipeline.Event{Kind: "stage.warn", Stage: string(store.StageSeparate),
				Code: "SEPARATE_UNAVAILABLE", Note: "vocal separation is not available yet; transcribing the original audio"})
			separate = false
			disabled[store.StageSeparate] = true
		}
	}

	cp, err := bundle.ReadCheckpoint(videoID)
	if err != nil {
		return out, fmt.Errorf("read checkpoint: %w", err)
	}
	// The hash the checkpoint records and the hash Plan compares must be the same
	// value, and it must never be empty: the write path substitutes a fallback for an
	// empty hash, so an empty value compared against that fallback and reported
	// "parameters changed" for identical settings.
	transcribeHash := params.HashInput()
	if transcribeHash == "" {
		transcribeHash = "engine-defaults"
	}
	stageParams := map[string]string{"transcribe": transcribeHash}

	// Plan only after every `disabled` decision is final, or the plan resumes at a
	// stage the runner then skips and the parameter comparison is never reached.
	plan := cp.Plan(bundle.Observe(), stageParams, disabled)
	for _, reason := range plan.Reasons {
		emitLogged(pipeline.Event{Kind: "plan.reason", Note: reason})
	}

	if _, isSynth := eng.(*SyntheticEngine); !isSynth {
		if err = PrepareAudio(ctx, layout, bundle, absInput, demux, PlanSkips(plan, store.StageDemux)); err != nil {
			return out, fmt.Errorf("prepare audio: %w", err)
		}
	}

	runner := &pipeline.Runner{Engine: eng, Emit: emitLogged}
	result, err := runner.Run(ctx, pipeline.Request{
		Bundle:       bundle,
		Plan:         plan,
		Params:       stageParams,
		VideoID:      videoID,
		Separate:     separate,
		Transcribe:   pipeline.TranscribeOptions{Model: modelID, VAD: params.VAD},
		SeparateOpts: pipeline.SeparateOptions{SegmentSecs: 15, OverlapSecs: 2, HalfPrecision: true},
	})
	if err != nil {
		meta := BuildMeta(videoID, o.Title, absInput, "failed", engName, engVersion, params)
		meta.Stages[string(result.FailedAt)] = store.StageRun{State: "failed", Message: err.Error()}
		_ = bundle.WriteMeta(meta)
		return out, fmt.Errorf("failed at %s: %w", result.FailedAt, err)
	}

	segs, err := pipeline.ReadTranscript(filepath.Join(bundle.Dir(), "text", "transcript.json"))
	if err != nil {
		return out, fmt.Errorf("read transcript: %w", err)
	}
	if err = WriteArtifacts(bundle, o.Title, segs); err != nil {
		return out, fmt.Errorf("export: %w", err)
	}

	meta := BuildMeta(videoID, o.Title, absInput, "done", engName, engVersion, params)
	for _, s := range result.Completed {
		meta.Stages[string(s)] = store.StageRun{State: "ok"}
	}
	if err = bundle.WriteMeta(meta); err != nil {
		return out, fmt.Errorf("write meta: %w", err)
	}

	out.Segments = len(segs)
	out.Completed = result.Completed
	return out, nil
}

// PlanSkips reports whether the plan considers a stage already valid. Callers need
// this before the pipeline starts: touching a file the checkpoint already validated
// invalidates its fingerprint and forces a needless re-run.
func PlanSkips(p store.Plan, s store.Stage) bool {
	for _, skip := range p.Skip {
		if skip == s {
			return true
		}
	}
	return false
}

// resolveModel keeps the requested model when it is installed, and otherwise
// substitutes the best installed whisper model. When nothing is installed it
// returns the request unchanged so the diagnosis can name exactly what to install.
func resolveModel(workdir string, reg registry.Registry, want string) (string, string) {
	if _, ok := engine.FindModel(workdir, reg, want); ok {
		return want, ""
	}
	best, bestRank := "", -1
	for _, m := range reg.Models {
		if m.Family != string(registry.EngineWhisper) {
			continue
		}
		if _, ok := engine.FindModel(workdir, reg, m.ID); !ok {
			continue
		}
		rank := map[string]int{"light": 1, "balanced": 2, "max": 3}[m.QualityTier]
		if rank > bestRank {
			best, bestRank = m.ID, rank
		}
	}
	if best == "" {
		return want, ""
	}
	return best, fmt.Sprintf(
		"%s is not installed; using the installed model %s. Install %s for higher quality.",
		want, best, want)
}

// LoadSettings resolves the workdir layout and settings.
func LoadSettings(wd string) (config.Settings, config.Layout, error) {
	layout, err := config.NewLayout(wd)
	if err != nil {
		return config.Settings{}, config.Layout{}, err
	}
	if err = layout.Ensure(); err != nil {
		return config.Settings{}, config.Layout{}, err
	}
	settings, err := layout.Load()
	if err != nil {
		return config.Settings{}, config.Layout{}, err
	}
	return settings, layout, nil
}

// VideoID derives the bundle id from the input path and size.
func VideoID(inputPath string) (string, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return "", err
	}
	return "local-" + store.HashParams(inputPath, fmt.Sprint(info.Size()))[4:], nil
}

// InputNeedsDemux reports whether the file must be converted before transcription.
func InputNeedsDemux(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav":
		return false
	default:
		return true
	}
}

var _ = registry.BackendCPU
