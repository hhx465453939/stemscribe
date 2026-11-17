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
type SeparateOptions struct {
	Model        string
	SegmentSecs  int
	OverlapSecs  int
	HalfPrecision bool
}

// Engine is the out-of-process inference port.
type Engine interface {
	Transcribe(ctx context.Context, audioPath, outPath string, opts TranscribeOptions, onSeg func(Segment)) error
	Separate(ctx context.Context, inPath, vocalsPath, instrumentalPath string, opts SeparateOptions) error
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
	Bundle   *store.Bundle
	Plan     store.Plan
	Separate bool
	Transcribe TranscribeOptions
	SeparateOpts SeparateOptions
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

	srcAudio, err := b.Path("audio/vocals.wav")
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

		switch stage {
		case store.StageSeparate:
			vocals, _ := b.Path("audio/vocals.wav")
			instr, _ := b.Path("audio/instrumental.wav")
			full, _ := b.Path("audio/full.wav")
			stageErr = r.Engine.Separate(ctx, full, vocals, instr, req.SeparateOpts)
		case store.StageTranscribe:
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

		missingOut, markErr := b.MarkDone(&cp, stage, store.HashParams(string(stage)))
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
		if len(out) > 0 && isDangling(out[len(out)-1]) && len([]rune(s.Text)) < DanglingMaxRunes {
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
