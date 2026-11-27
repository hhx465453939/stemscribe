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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	Input   string
	Title   string
	Workdir string
	ModelID string
	// SeparatorID overrides the configured separation model for this run only.
	// Every model family (MDX, VR, Demucs 4/6-stem) is selected the same way, so
	// the CLI can exercise a model before the model centre exists for it.
	SeparatorID string
	Engine      string // auto (real whisper.cpp) | synthetic (test double)
	DurationMS  int
	Params      engine.WhisperParams
	Separate    bool
	// SepTuning overrides the separation knobs for this run. The zero value keeps
	// each architecture's upstream default, which is what a caller with no
	// opinion should get.
	SepTuning SeparateTuning
	// ExportStems names the stems to keep after separation. Empty means "keep
	// everything the model produces". vocals is mandatory -- transcription reads
	// it -- and every name must belong to the selected model; anything else is
	// rejected before the run starts rather than silently ignored.
	ExportStems []string
	// OutputDir overrides Settings.OutputDir for this run only. Empty means "use the
	// configured default", which itself falls back to workdir/libraries.
	OutputDir string
	// Emit receives pipeline events. All progress is derived from these; nothing is
	// invented by a timer (ADR-023).
	Emit func(pipeline.Event)

	// retryDowngraded guards the one automatic retry after an out-of-memory
	// failure, so a machine that cannot run the fallback either fails instead of
	// looping through every model.
	retryDowngraded bool
	// downgradeNote explains, inside the retried run's own log, why the model is
	// not the requested one.
	downgradeNote string
}

// SeparateTuning is the per-run separation parameter set.
//
// Units follow the architecture, not this struct: Segment is STFT frames for
// MDX/MDXC and seconds for Demucs. A value that the selected architecture does
// not honour is rejected here rather than silently ignored deeper in Python.
type SeparateTuning struct {
	Segment   int
	Overlap   float64
	Shifts    int
	BatchSize int
	// FP16 asks for torch autocast (CUDA only). Off by default: the quality has
	// to be measured per model before it can become a default.
	FP16 bool
}

// Validate rejects combinations the selected model cannot honour, so the user
// hears about it before a GPU run starts instead of in a Python traceback.
func (t SeparateTuning) Validate(m registry.Model) error {
	if t.Segment < 0 || t.Overlap < 0 || t.Overlap >= 1 || t.Shifts < 0 || t.BatchSize < 0 {
		return fmt.Errorf("separation parameters out of range (segment/shifts/batch must be >= 0, overlap in [0,1))")
	}
	if t.Segment > 0 && !m.Params.SegmentSize {
		return fmt.Errorf("%s (%s) has no segment_size setting", m.ID, m.Architecture)
	}
	if t.Overlap > 0 && !m.Params.Overlap {
		return fmt.Errorf("%s (%s) has no overlap setting", m.ID, m.Architecture)
	}
	if t.Shifts > 0 && !m.Params.Shifts {
		return fmt.Errorf("%s (%s) has no shifts setting", m.ID, m.Architecture)
	}
	if t.BatchSize > 0 && !m.Params.BatchSize {
		return fmt.Errorf("%s (%s) has no batch_size setting", m.ID, m.Architecture)
	}
	return nil
}

// Outcome reports what a finished run produced.
type Outcome struct {
	VideoID   string
	BundleDir string
	Segments  int
	Completed []store.Stage
}

// TotalStages is the progress denominator: the stages this run will actually
// report. Polish is never part of the local path; separation counts whenever the
// run is real, because it is a mandatory stage there (ADR-037).
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

// separationEnabled reports whether a run must extract vocals before it
// transcribes. Only the synthetic test double is exempt: it has no audio in the
// first place, so demanding a UVR5 runtime from it would only break the smoke test.
func separationEnabled(kind string) bool {
	return !strings.EqualFold(kind, "synthetic")
}

// Run executes one bundle end to end. It returns an error only for fatal
// conditions; a stage failure is reported through the error it carries.
//
// One automatic retry is built in: if the engine ran out of device memory the
// run is restarted with the next smaller installed model and the substitution is
// written to the task's own event log (ADR-039). Demux and separation are already
// checkpointed, so the retry resumes at transcribe instead of redoing the work.
func Run(ctx context.Context, o Options) (Outcome, error) {
	out, err := runOnce(ctx, o)
	if err == nil || o.retryDowngraded || !isAllocationFailure(err) {
		return out, err
	}
	wd := o.Workdir
	if wd == "" {
		if wd, err = config.DefaultWorkdir(); err != nil {
			return out, err
		}
	}
	requested := o.ModelID
	if requested == "" {
		if settings, _, loadErr := LoadSettings(wd); loadErr == nil {
			requested = settings.Stages.Whisper
		}
	}
	next, ok := nextSmallerInstalled(wd, requested)
	if !ok {
		return out, err
	}
	retry := o
	retry.ModelID = next
	retry.retryDowngraded = true
	retry.downgradeNote = fmt.Sprintf(
		"%s ran out of memory; retried with %s. Install more VRAM headroom or pick %s to avoid this.",
		requested, next, next)
	retryOut, retryErr := runOnce(ctx, retry)
	if retryErr != nil {
		return retryOut, retryErr
	}
	return retryOut, nil
}

// runOnce is the real body of a run.
func runOnce(ctx context.Context, o Options) (Outcome, error) {
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
	bundleRoot := layout.Libraries()
	if dir := o.OutputDir; dir != "" {
		bundleRoot = dir
	} else if settings.OutputDir != "" {
		bundleRoot = settings.OutputDir
	}
	// One video, one self-contained directory, wherever the user asked for it. Products
	// flattened into a shared folder would collide and could not be moved as a unit.
	bundle, err := store.New(filepath.Join(bundleRoot, videoID))
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

	// Separation is mandatory for the real engine and never inferred from a user
	// switch (ADR-037). Only the synthetic test double -- which cannot load a model
	// at all -- runs without it.
	separate := separationEnabled(kind)
	disabled := map[store.Stage]bool{
		store.StageSeparate: !separate,
		store.StagePolish:   true,
	}

	modelID := o.ModelID
	if modelID == "" {
		modelID = settings.Stages.Whisper
	}
	sepModelID := ""
	if separate {
		sepModelID = o.SeparatorID
		if sepModelID == "" {
			sepModelID = settings.Stages.Separator
		}
		if sepModelID == "" {
			sepModelID = config.Defaults(layout.Root).Stages.Separator
		}
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
	sepArch := ""
	if separate {
		reg, regErr := registry.Builtin()
		if regErr != nil {
			return out, fmt.Errorf("load registry: %w", regErr)
		}
		m, ok := reg.FindModel(sepModelID)
		if !ok {
			return out, fmt.Errorf("unknown separation model %q", sepModelID)
		}
		sepArch = string(m.Architecture)
		if err := o.SepTuning.Validate(m); err != nil {
			return out, err
		}
		if len(o.ExportStems) > 0 {
			keep, kerr := normalizeStems(o.ExportStems, m)
			if kerr != nil {
				return out, kerr
			}
			o.ExportStems = keep
		}
	}
	engName, engVersion := "synthetic", Version
	eng, engDesc, engErr := BuildEngine(kind, layout.Root, modelID, sepModelID,
		string(settings.Runtime.LockedBackend), o.DurationMS, demux, params)
	if engErr != nil {
		return out, engErr
	}
	emitLogged(pipeline.Event{Kind: "engine", Note: engDesc})
	if o.downgradeNote != "" {
		emitLogged(pipeline.Event{Kind: "stage.warn", Code: "MODEL_SUBSTITUTED", Note: o.downgradeNote})
	}
	sepVersion := ""
	backendName := ""
	switch e := eng.(type) {
	case *engine.Composite:
		engName, engVersion = "whisper.cpp", e.Version
		backendName = e.Backend
		if e.Separator != nil {
			sepVersion = e.Separator.Version
		}
	case *engine.Whisper:
		engName, engVersion = "whisper.cpp", e.Version
		backendName = e.Backend
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
	// The model is part of the stage's identity: without this a switch from medium
	// to large-v3 resumed at align and reused the old transcript, which made the
	// model option look like it did nothing.
	stageParams := map[string]string{
		"transcribe": transcribeHash + ";model=" + modelID,
		"separate":   separationHash(sepModelID, o.SepTuning, o.ExportStems),
	}

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
		Bundle:     bundle,
		Plan:       plan,
		Params:     stageParams,
		VideoID:    videoID,
		Separate:   separate,
		Transcribe: pipeline.TranscribeOptions{Model: modelID, VAD: params.VAD},
		SeparateOpts: pipeline.SeparateOptions{
			Model: sepModelID, Arch: sepArch,
			Segment: o.SepTuning.Segment, Overlap: o.SepTuning.Overlap,
			Shifts: o.SepTuning.Shifts, BatchSize: o.SepTuning.BatchSize,
			HalfPrecision: o.SepTuning.FP16,
			Stems:         separatorStems(sepModelID),
			Keep:          o.ExportStems,
		},
	})
	if err != nil {
		// A stop is a user decision, not a defect: record it as canceled so the
		// library does not list it among failures. The stage is still marked so a
		// resumed run knows where it left off.
		status, stageState := "failed", "failed"
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			status, stageState = "canceled", "canceled"
		}
		meta := BuildMeta(videoID, o.Title, absInput, status, engName, engVersion, backendName, modelID, sepModelID, sepVersion, separatorBackend(eng), params)
		recordSeparateRuntime(&meta, eng, bundle, o.ExportStems)
		meta.Stages[string(result.FailedAt)] = store.StageRun{State: stageState, Message: err.Error()}
		_ = bundle.WriteMeta(meta)
		return out, fmt.Errorf("failed at %s: %w", result.FailedAt, err)
	}

	segs, err := pipeline.ReadTranscript(filepath.Join(bundle.Dir(), "text", "transcript.json"))
	if err != nil {
		return out, fmt.Errorf("read transcript: %w", err)
	}
	// Separation succeeded; audio/ now holds this run's stems. Anything else with
	// a .wav name is a leftover from an earlier model choice (switching UVR5 ->
	// Demucs left instrumental.wav behind). The UI promises unselected stems are
	// gone, so an orphan from a previous run must not survive either. Sweep when
	// separation is enabled at all: a stage skipped as "already valid" still owns
	// a well-defined live set, while a run with separation disabled must not
	// delete files as a side effect of not separating.
	if separate {
		stale, err := dropStaleAudio(bundle, o.ExportStems, sepModelID)
		if err != nil {
			return out, fmt.Errorf("drop stale audio: %w", err)
		}
		if len(stale) > 0 {
			emitLogged(pipeline.Event{Kind: "stems", Stage: string(store.StageSeparate),
				Note: "removed stale=" + strings.Join(stale, ",")})
		}
	}
	if err = WriteArtifacts(bundle, o.Title, segs); err != nil {
		return out, fmt.Errorf("export: %w", err)
	}

	meta := BuildMeta(videoID, o.Title, absInput, "done", engName, engVersion, backendName, modelID, sepModelID, sepVersion, separatorBackend(eng), params)
	recordSeparateRuntime(&meta, eng, bundle, o.ExportStems)
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

// separatorBackend reports the device the separator actually used in this run.
// Empty when the separator never ran (for example a failure during demux), which
// is left empty rather than guessed.
func separatorBackend(eng pipeline.Engine) string {
	if c, ok := eng.(*engine.Composite); ok && c.Separator != nil {
		return c.Separator.LastBackend
	}
	return ""
}

// modelLadder orders the whisper models from largest to smallest. A downgrade
// walks it downwards; it is deliberately a fixed list rather than a sort on
// registry fields, so the order is reviewable in one place.
var modelLadder = []string{"whisper-large-v3", "whisper-medium", "whisper-small", "whisper-base", "whisper-tiny"}

// isAllocationFailure recognises the errors whisper.cpp/ggml emit when the model
// does not fit on the device. The strings are the engine's own words; matching a
// generic "error" here would retry on every crash.
func isAllocationFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"out of memory",
		"failed to allocate",
		"allocation failed",
		"erroroutofdevicememory",
		"vk::device::allocatememory",
		"device memory allocation",
		"not enough memory",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// nextSmallerInstalled returns the next model down the ladder that is actually on
// disk, so a retry never fails on a missing file.
func nextSmallerInstalled(workdir, current string) (string, bool) {
	reg, err := registry.Builtin()
	if err != nil {
		return "", false
	}
	idx := -1
	for i, id := range modelLadder {
		if id == current {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", false
	}
	for _, id := range modelLadder[idx+1:] {
		if _, ok := engine.FindModel(workdir, reg, id); ok {
			return id, true
		}
	}
	return "", false
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

// normalizeStems validates a requested retention set against the model's declared
// output and returns it sorted and de-duplicated.
//
// vocals is required because transcription reads it; a set without vocals would
// produce a bundle that cannot be transcribed, and failing after a multi-minute
// separation is worse than refusing at the door.
func normalizeStems(want []string, m registry.Model) ([]string, error) {
	allowed := make(map[string]bool, len(m.Stems))
	for _, s := range m.Stems {
		allowed[s] = true
	}
	seen := make(map[string]bool, len(want))
	out := make([]string, 0, len(want))
	var unknown []string
	for _, raw := range want {
		s := strings.TrimSpace(raw)
		if s == "" || seen[s] {
			continue
		}
		if !allowed[s] {
			unknown = append(unknown, s)
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("model %q has no stem(s) %s; available: %s",
			m.ID, strings.Join(unknown, ", "), strings.Join(m.Stems, ", "))
	}
	if !seen["vocals"] {
		return nil, fmt.Errorf("the stem set must include \"vocals\": transcription reads it")
	}
	sort.Strings(out)
	return out, nil
}

// separatorStems is the configured model's declared output set. An id the
// catalog does not know yields nil, and the pipeline then requires only vocals.
func separatorStems(modelID string) []string {
	reg, err := registry.Builtin()
	if err != nil {
		return nil
	}
	if m, ok := reg.FindModel(modelID); ok {
		return m.Stems
	}
	return nil
}

// separationHash is the checkpoint key for the separate stage. It has to cover
// every knob that changes the output: changing shifts or overlap while the model
// id stays the same, then resuming, used to keep the stems produced with the old
// values. The retained-stem set belongs here too: unchecking drums and resuming
// must re-run separation instead of being treated as "already done".
func separationHash(modelID string, t SeparateTuning, keep []string) string {
	kept := "all"
	if len(keep) > 0 {
		sorted := append([]string(nil), keep...)
		sort.Strings(sorted)
		kept = strings.Join(sorted, ",")
	}
	return store.HashParams(
		"model="+modelID,
		fmt.Sprintf("segment=%d", t.Segment),
		fmt.Sprintf("overlap=%g", t.Overlap),
		fmt.Sprintf("shifts=%d", t.Shifts),
		fmt.Sprintf("batch=%d", t.BatchSize),
		fmt.Sprintf("fp16=%v", t.FP16),
		"keep="+kept,
	)
}

// audioDerived names the wavs in audio/ that are not stems: the source copy,
// its 16 kHz form, and the 16 kHz vocals copy transcription consumes. Stem
// retention never applies to them, and stale sweeping must never touch them.
var audioDerived = map[string]bool{"full.wav": true, "16k.wav": true, "vocals16k.wav": true}

// dropStaleAudio removes stem files left behind by an earlier separation when
// the current settings produce a different set. pruneStems only sees the stems
// this run produced, so a bundle reused across model changes kept orphans such
// as instrumental.wav from an old UVR5 run while the UI claimed they were gone.
//
// The sweep is deliberately conservative: it acts only on *.wav directly under
// audio/, never when the live stem set is unknown (an unrecognised model must
// not turn "keep nothing" into "delete everything"), and never on derived files.
func dropStaleAudio(bundle *store.Bundle, keep []string, sepModelID string) ([]string, error) {
	dir, err := bundle.Path("audio")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	live := make(map[string]bool, len(keep)+len(audioDerived))
	for _, stem := range keep {
		live[stem+".wav"] = true
	}
	if len(keep) == 0 {
		for _, stem := range separatorStems(sepModelID) {
			live[stem+".wav"] = true
		}
	}
	if len(live) == 0 {
		return nil, nil
	}
	var removed []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".wav") || live[name] || audioDerived[name] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("remove %q: %w", name, err)
		}
		removed = append(removed, strings.TrimSuffix(name, ".wav"))
	}
	sort.Strings(removed)
	return removed, nil
}

// recordSeparateRuntime adds the facts only the engine knows after a run: the
// inference stack and the stem files that really exist on disk. Candidates are the
// retained set (or the model's full declaration when nothing was pruned), then
// filtered by os.Stat -- a run that failed mid-separation must not claim stems it
// never wrote.
func recordSeparateRuntime(meta *store.Meta, eng pipeline.Engine, bundle *store.Bundle, kept []string) {
	c, ok := eng.(*engine.Composite)
	if !ok || c.Separator == nil {
		return
	}
	info, ok := meta.Provenance.Engines["separate"]
	if !ok {
		return
	}
	info.Runtime = c.Separator.LastRuntime
	candidates := kept
	if len(candidates) == 0 {
		candidates = separatorStems(info.ModelID)
	}
	present := make([]string, 0, len(candidates))
	for _, stem := range candidates {
		path, err := bundle.Path("audio/" + stem + ".wav")
		if err != nil {
			continue
		}
		if _, statErr := os.Stat(path); statErr == nil {
			present = append(present, stem)
		}
	}
	sort.Strings(present)
	info.Stems = present
	meta.Provenance.Engines["separate"] = info
}
