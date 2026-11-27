// Package pipeline assembles the stage DAG and the alignment repairs that make a
// transcript trustworthy (PIPELINE.md §2/§4).
//
// Inference itself lives behind the Engine port (ADR-002); this package must stay
// testable with a fake engine so CI can verify the orchestration without a GPU.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/store"
)

// Segment is one transcribed line.
type Segment struct {
	StartMS    int     `json:"start_ms"`
	EndMS      int     `json:"end_ms"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	LowConf    bool    `json:"low_conf,omitempty"`
}

// TranscribeOptions are the knobs passed to the engine.
type TranscribeOptions struct {
	Model     string
	Language  string
	Backend   string
	GPULayers int
	VAD       bool
	Threads   int
}

// SeparateOptions are the knobs passed to the separator.
// SeparateOptions describes one separation request.
//
// The knobs below are architecture specific and their units differ on purpose:
// MDX-Net counts segment_size in STFT frames (256 is the upstream default) while
// Demucs counts it in seconds ("Default" means the model's native 7.8 s). The
// adapter maps each field onto the right architecture dictionary, and a field
// that the architecture does not honour is never sent -- audio-separator would
// silently ignore it and the user would think the setting did something.
type SeparateOptions struct {
	Model string
	// Segment is segment_size in the units of the model's architecture (see
	// above). Zero means "leave the upstream default".
	Segment int
	// Overlap is the window overlap as a fraction (0.25 upstream default).
	// Zero means "leave the upstream default".
	Overlap float64
	// HalfPrecision asks the torch stack for fp16 autocast. It is opt-in: the
	// quality has to be measured per model before it becomes a default.
	HalfPrecision bool
	// Shifts and BatchSize are architecture-specific knobs (Demucs shifts,
	// MDX/VR/MDXC batch). Zero means "leave the model default".
	Shifts    int
	BatchSize int
	// Arch is the registry architecture of Model (mdx/vr/demucs/mdxc/roformer).
	// The adapter needs it to pick the parameter dictionary; inferring it from
	// the filename is how a VR model ends up with MDX parameters.
	Arch string
	// Stems is the set the configured model must produce, from the registry. A
	// missing stem is a failed separation, never a quietly shorter result.
	Stems []string
	// Keep selects which produced stems are retained on disk. Empty means "keep
	// everything". The model always computes every track (audio-separator has no
	// single-stem mode), so this is a disk decision: unselected stems are removed
	// after they were written, before the checkpoint lists the outputs.
	Keep []string
}

// Engine is the out-of-process inference port.
type Engine interface {
	Transcribe(ctx context.Context, audioPath, outPath string, opts TranscribeOptions, onSeg func(Segment)) error
	// Separate writes <stem>.wav files into outDir and returns their absolute
	// paths. Which stems exist is the model's property, not the caller's, so the
	// port returns the set instead of naming two fixed outputs.
	Separate(ctx context.Context, inPath, outDir string, opts SeparateOptions) (map[string]string, error)
}

// Event kinds written to runs/<run_id>.jsonl.
type Event struct {
	TS    string            `json:"ts"`
	Kind  string            `json:"ev"`
	Stage string            `json:"stage,omitempty"`
	OK    *bool             `json:"ok,omitempty"`
	Code  string            `json:"code,omitempty"`
	Note  string            `json:"note,omitempty"`
	Extra map[string]string `json:"extra,omitempty"`
}

// Emitter receives progress events.
type Emitter func(Event)

// Runner executes the DAG for one episode.
type Runner struct {
	Engine Engine
	Emit   Emitter
}

// Request describes what to run.
type Request struct {
	Bundle       *store.Bundle
	Plan         store.Plan
	Separate     bool
	Transcribe   TranscribeOptions
	SeparateOpts SeparateOptions
	// Params carries the real parameter hash per stage, computed by the caller
	// (only it knows the engine settings). Without this the checkpoint recorded
	// HashParams(stage) -- a constant -- so changing beam_size could never
	// invalidate a transcript, and the invalidation branch was dead code.
	Params map[string]string
	// VideoID is used for checkpoint bookkeeping.
	VideoID string
}

// Result reports what happened.
type Result struct {
	Completed []store.Stage
	FailedAt  store.Stage
	Err       error
}

// Run executes the plan.
//
// Only the stages that exist in the skeleton are executed; download/probe/demux
// are performed by the caller (or by the CLI) because they need ffmpeg. Stages
// outside the plan are skipped and recorded as such, never silently "ok".
func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	if r.Engine == nil {
		return Result{}, fmt.Errorf("engine port is nil")
	}
	if req.Bundle == nil {
		return Result{}, fmt.Errorf("bundle is nil")
	}

	b := req.Bundle
	if err := b.Ensure(); err != nil {
		return Result{}, err
	}

	cp, err := b.ReadCheckpoint(req.VideoID)
	if err != nil {
		return Result{}, err
	}

	res := Result{}
	emit := func(e Event) {
		if r.Emit != nil {
			r.Emit(e)
		}
	}

	srcAudio, err := b.Path("audio/vocals16k.wav")
	if err != nil {
		return res, err
	}
	fallbackAudio, err := b.Path("audio/16k.wav")
	if err != nil {
		return res, err
	}
	transcriptPath, err := b.Path("text/transcript.json")
	if err != nil {
		return res, err
	}

	for _, stage := range store.StageOrder {
		if ctxErr := ctx.Err(); ctxErr != nil {
			res.FailedAt = stage
			res.Err = ctxErr
			return res, ctxErr
		}
		if !stageEnabled(stage, req) {
			emit(Event{Kind: "stage.skip", Stage: string(stage), Note: "disabled by settings"})
			continue
		}
		if planSkipped(req.Plan, stage) {
			emit(Event{Kind: "stage.skip", Stage: string(stage), Note: "already valid"})
			res.Completed = append(res.Completed, stage)
			continue
		}

		emit(Event{Kind: "stage.start", Stage: string(stage)})
		var stageErr error
		var sepStems map[string]string

		switch stage {
		case store.StageSeparate:
			audioDir, _ := b.Path("audio")
			full, _ := b.Path("audio/full.wav")
			sepStems, stageErr = r.Engine.Separate(ctx, full, audioDir, req.SeparateOpts)
			if stageErr == nil {
				stageErr = requireStems(sepStems, req.SeparateOpts.Stems)
			}
			if stageErr == nil && len(req.SeparateOpts.Keep) > 0 {
				var dropped []string
				sepStems, dropped, stageErr = pruneStems(sepStems, req.SeparateOpts.Keep)
				if stageErr == nil && len(dropped) > 0 {
					// Auditable: the bundle says which stems it kept, and the run log
					// records what was thrown away instead of leaving the user to guess.
					emit(Event{Kind: "stems", Stage: string(stage),
						Note: "kept=" + strings.Join(sortedKeys(sepStems), ",") +
							" dropped=" + strings.Join(dropped, ",")})
				}
			}
		case store.StageTranscribe:
			// Transcribe the 16 kHz copy of the separated vocals. Falling back to the
			// source's 16k.wav happens only when separation did not run (the synthetic
			// test double); a real run with a missing vocals track fails earlier.
			in := srcAudio
			if _, statErr := os.Stat(in); statErr != nil {
				in = fallbackAudio
			}
			stageErr = r.Engine.Transcribe(ctx, in, transcriptPath, req.Transcribe, func(Segment) {})
		case store.StageAlign:
			var segs []Segment
			segs, stageErr = ReadTranscript(transcriptPath)
			if stageErr == nil {
				segs = AlignRepair(segs)
				stageErr = WriteTranscript(transcriptPath, segs)
			}
		default:
			// resolve/download/probe/demux/export are no-ops in the skeleton; they
			// are owned by CHG-0003 where file handling lands with ffmpeg.
			stageErr = nil
		}

		if stageErr != nil {
			ok := false
			emit(Event{Kind: "stage.end", Stage: string(stage), OK: &ok, Note: stageErr.Error()})
			res.FailedAt = stage
			res.Err = stageErr
			return res, stageErr
		}

		outputs := store.Outputs(stage)
		if stage == store.StageSeparate {
			outputs = separateOutputs(sepStems)
		}
		missingOut, markErr := b.MarkDoneWith(&cp, stage, stageParamHash(req.Params, stage), outputs)
		if markErr != nil {
			return res, markErr
		}
		if len(missingOut) > 0 {
			// Not fatal, but never silent: the next Plan() run will resume here.
			emit(Event{Kind: "stage.warn", Stage: string(stage), Code: "MISSING_OUTPUT",
				Note: strings.Join(missingOut, ",")})
		}
		if err = b.WriteCheckpoint(cp); err != nil {
			return res, err
		}

		ok := true
		emit(Event{Kind: "stage.end", Stage: string(stage), OK: &ok})
		res.Completed = append(res.Completed, stage)
	}
	return res, nil
}

func stageEnabled(s store.Stage, req Request) bool {
	if s == store.StageSeparate {
		return req.Separate
	}
	if s == store.StagePolish {
		// Polishing is opt-in and never part of the local-only path (N-05).
		return false
	}
	return true
}

// stageParamHash returns the caller-supplied hash for a stage, falling back to a
// stage-derived constant only when the caller supplied nothing. The fallback keeps
// old bundles readable; it must never be the normal path, because a constant hash
// makes parameter changes invisible to Plan().
func stageParamHash(params map[string]string, s store.Stage) string {
	if h, ok := params[string(s)]; ok && h != "" {
		return h
	}
	return store.HashParams(string(s))
}

func planSkipped(p store.Plan, s store.Stage) bool {
	for _, skip := range p.Skip {
		if skip == s {
			return true
		}
	}
	return false
}

// ReadTranscript loads segments from disk.
func ReadTranscript(path string) ([]Segment, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}
	return parseSegments(raw)
}

// WriteTranscript persists segments atomically.
func WriteTranscript(path string, segs []Segment) error {
	raw, err := marshalSegments(segs)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create transcript dir: %w", err)
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write transcript: %w", err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("install transcript: %w", err)
	}
	return nil
}

// AlignRepair applies the three fixes from PIPELINE.md §4.3:
//  1. merge sentences split across segment boundaries
//  2. drop whisper hallucination loops (repeated text)
//  3. mark low-confidence segments instead of deleting them
func AlignRepair(segs []Segment) []Segment {
	out := make([]Segment, 0, len(segs))
	for _, s := range segs {
		s.Text = strings.TrimSpace(s.Text)
		if s.Text == "" {
			continue
		}
		// Fix 3: mark, never drop — a deleted line is unrecoverable.
		if s.Confidence > 0 && s.Confidence < LowConfidenceThreshold {
			s.LowConf = true
		}
		// Fix 1: merge a dangling fragment into the previous segment.
		//
		// The accumulated length must be bounded. isDangling already stops a merge
		// after sentence-final punctuation, but Chinese speech often runs tens of
		// seconds without any, so an unbounded append glued a whole paragraph onto one
		// line: measured median 32 / p90 96 / max 328 runes on a 30-minute episode,
		// which cannot be used as subtitles. whisper itself emits ~10-rune fragments,
		// so the ceiling has to be applied here, not in the engine flags.
		if len(out) > 0 && isDangling(out[len(out)-1]) && len([]rune(s.Text)) < DanglingMaxRunes &&
			len([]rune(out[len(out)-1].Text))+len([]rune(s.Text)) <= MaxMergedRunes {
			prev := &out[len(out)-1]
			prev.Text = strings.TrimSpace(prev.Text + s.Text)
			prev.EndMS = s.EndMS
			if s.Confidence > 0 && (prev.Confidence == 0 || s.Confidence < prev.Confidence) {
				prev.Confidence = s.Confidence
			}
			continue
		}
		out = append(out, s)
	}
	// Fix 2: collapse hallucination loops. Keep the first occurrence, drop the
	// repeats; repeat runs are a known whisper failure mode on silence.
	return dropHallucinationRuns(out)
}

// LowConfidenceThreshold below which a segment is flagged for review.
const LowConfidenceThreshold = 0.45

// DanglingMaxRunes is the upper bound for a fragment worth merging upstream.
const DanglingMaxRunes = 12

// MaxMergedRunes caps how long a merged segment may become.
//
// 40 runes is the point where a Chinese subtitle line still reads as one line on a
// phone at the UI-DESIGN type scale, and it is above the 20-30 rune target in the
// PRD so ordinary sentences survive intact. Without this ceiling a single line grew
// to 328 runes (see docs/GAP-ANALYSIS.md §1).
const MaxMergedRunes = 40

// HallucinationRepeatCount is how many near-identical neighbours count as a loop.
const HallucinationRepeatCount = 3

func isDangling(s Segment) bool {
	t := strings.TrimSpace(s.Text)
	if t == "" {
		return false
	}
	last := []rune(t)[len([]rune(t))-1]
	return !strings.ContainsRune("。！？.!?…；;」』", last)
}

func dropHallucinationRuns(segs []Segment) []Segment {
	if len(segs) < HallucinationRepeatCount {
		return segs
	}
	out := make([]Segment, 0, len(segs))
	i := 0
	for i < len(segs) {
		// Measure the whole run first: deciding while scanning kept two copies
		// instead of "keep the first" (PIPELINE.md §4.3).
		j := i + 1
		for j < len(segs) && similarText(segs[j-1].Text, segs[j].Text) && segs[j].StartMS-segs[j-1].EndMS < 1000 {
			j++
		}
		if j-i >= HallucinationRepeatCount {
			out = append(out, segs[i])
		} else {
			out = append(out, segs[i:j]...)
		}
		i = j
	}
	return out
}

// similarText reports near-duplicate text (cheap normalised equality).
func similarText(a, b string) bool {
	na := normalise(a)
	nb := normalise(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	// Prefix containment catches whisper's "sentence + extra words" loops.
	shorter, longer := na, nb
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	return len(shorter) >= 6 && strings.HasPrefix(longer, shorter)
}

func normalise(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '，', '。', '！', '？', '、', '；', '：', ' ', '\t', '\n', ',', '.', '!', '?', ';', ':':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// requireStems checks that every stem the registry promised exists in the
// result. A 4-stem model that returns three is a failed separation, not a
// successful one with fewer files: the UI would otherwise offer a bundle whose
// drums track silently never existed.
func requireStems(got map[string]string, want []string) error {
	if len(want) == 0 {
		if _, ok := got["vocals"]; !ok {
			return fmt.Errorf("separation produced no vocals track")
		}
		return nil
	}
	var missing []string
	for _, stem := range want {
		if got[stem] == "" {
			missing = append(missing, stem)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("separation did not produce required stem(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

// pruneStems removes every produced stem that is not in keep and returns the
// retained map plus the sorted names that were dropped.
//
// Separation always computes all tracks; there is no single-stem mode upstream.
// Deleting afterwards is therefore a disk decision, and it has to happen before
// the checkpoint records outputs, or the next plan would look for files this run
// deliberately removed.
func pruneStems(stems map[string]string, keep []string) (map[string]string, []string, error) {
	wanted := make(map[string]bool, len(keep))
	for _, name := range keep {
		wanted[name] = true
	}
	kept := make(map[string]string, len(stems))
	var dropped []string
	for name, path := range stems {
		if wanted[name] {
			kept[name] = path
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("drop stem %q: %w", name, err)
		}
		dropped = append(dropped, name)
	}
	sort.Strings(dropped)
	return kept, dropped, nil
}

// sortedKeys returns the map's keys in a stable order, for log lines that must
// not change between identical runs.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// separateOutputs lists the bundle-relative files a separation actually wrote.
func separateOutputs(stems map[string]string) []string {
	names := make([]string, 0, len(stems)+1)
	for stem := range stems {
		names = append(names, "audio/"+stem+".wav")
	}
	names = append(names, "audio/vocals16k.wav")
	sort.Strings(names)
	return names
}
