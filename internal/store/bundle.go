// Package store owns the on-disk bundle layout and the resumability rules
// (ADR-006).
//
// The single most important function here is Checkpoint.Plan: it decides which
// stage to restart from. Getting it wrong is expensive in both directions —
// too eager and the user re-runs hours of GPU work, too lax and a stale artifact
// is silently kept.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hhx465453939/stemscribe/internal/fsutil"
)

// Stage names one step of the pipeline (PIPELINE.md §1).
type Stage string

const (
	StageResolve    Stage = "resolve"
	StageDownload   Stage = "download"
	StageProbe      Stage = "probe"
	StageDemux      Stage = "demux"
	StageSeparate   Stage = "separate"
	StageTranscribe Stage = "transcribe"
	StageAlign      Stage = "align"
	StagePolish     Stage = "polish"
	StageExport     Stage = "export"
)

// StageOrder is the canonical execution order.
var StageOrder = []Stage{
	StageResolve, StageDownload, StageProbe, StageDemux,
	StageSeparate, StageTranscribe, StageAlign, StagePolish, StageExport,
}

// stageOutputs maps a stage to the bundle-relative files it produces. Used to
// detect that a user (or a crashed run) left an inconsistent bundle behind.
//
// These must match what the stages actually produce. An earlier version listed
// audio/full.wav for probe and demux, which the real path never writes (audio goes
// straight to a whisper-ready 16k wav), so every run logged MISSING_OUTPUT and
// recorded fingerprints for files that did not exist -- which then made the next
// resume decision re-run stages that were already valid.
var stageOutputs = map[Stage][]string{
	StageResolve:  {"meta.json"},
	StageDownload: {"source/video.mp4"},
	// Probe only reads metadata; it writes nothing of its own.
	StageProbe: nil,
	// prepareAudio writes this file before the pipeline runs (copy or ffmpeg).
	StageDemux:      {"audio/16k.wav"},
	StageSeparate:   {"audio/vocals.wav"},
	StageTranscribe: {"text/transcript.json"},
	StageAlign:      {"text/transcript.json"},
	StagePolish:     {"text/polished.md"},
	// The CLI owns subtitle/markdown export and writes them after the run; a
	// failure there aborts the run before meta.json is marked done, so nothing is
	// silently lost. Declaring outputs here would fingerprint files that do not
	// exist yet at the moment the stage completes.
	StageExport: nil,
}

// Outputs returns the files a stage must leave behind.
func Outputs(s Stage) []string { return stageOutputs[s] }

// Fingerprint detects out-of-band modification of a stage output.
type Fingerprint struct {
	Size    int64 `json:"size"`
	ModTime int64 `json:"mod_time"`
}

// Checkpoint is the persisted resumability state (.state.json).
type Checkpoint struct {
	Schema    int                    `json:"schema"`
	VideoID   string                 `json:"video_id"`
	Completed []Stage                `json:"completed"`
	Params    map[string]string      `json:"params_hash"`
	Files     map[string]Fingerprint `json:"files"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// NewCheckpoint returns an empty checkpoint.
func NewCheckpoint(videoID string) Checkpoint {
	return Checkpoint{
		Schema:  1,
		VideoID: videoID,
		Params:  map[string]string{},
		Files:   map[string]Fingerprint{},
	}
}

// Plan is the resume decision.
type Plan struct {
	// ResumeAt is the first stage that must run. Empty means "everything is done".
	ResumeAt Stage
	// Reasons explains each invalidation, for the run log and the UI.
	Reasons []string
	// Skip lists stages that are already valid.
	Skip []Stage
}

// Plan decides where to resume.
//
// observed holds the current on-disk fingerprint per bundle-relative path, and
// currentParams the parameter hash per stage. disabled names stages the user
// turned off (e.g. separation): those are neither "done" nor "pending" and must
// not block stages after them.
//
// A stage is reusable only if it was completed, its parameters are unchanged, and
// every output that was **recorded** still matches. Paths with no recorded
// fingerprint are skipped: a checkpoint that never fingerprinted an output has no
// evidence either way, and inventing a failure there would force full re-runs.
func (cp Checkpoint) Plan(observed map[string]Fingerprint, currentParams map[string]string, disabled map[Stage]bool) Plan {
	done := make(map[Stage]bool, len(cp.Completed))
	for _, s := range cp.Completed {
		done[s] = true
	}

	var plan Plan
	for _, s := range StageOrder {
		if disabled[s] {
			continue
		}
		if !done[s] {
			plan.ResumeAt = s
			plan.Reasons = append(plan.Reasons, fmt.Sprintf("%s: not completed", s))
			return plan
		}
		if want, ok := cp.Params[string(s)]; ok {
			if got, ok2 := currentParams[string(s)]; ok2 && got != want {
				plan.ResumeAt = s
				plan.Reasons = append(plan.Reasons, fmt.Sprintf("%s: parameters changed", s))
				return plan
			}
		}
		for _, rel := range stageOutputs[s] {
			recorded, hadRecord := cp.Files[rel]
			if !hadRecord {
				continue
			}
			current, exists := observed[rel]
			if !exists {
				plan.ResumeAt = s
				plan.Reasons = append(plan.Reasons, fmt.Sprintf("%s: output %s missing", s, rel))
				return plan
			}
			if recorded != current {
				plan.ResumeAt = s
				plan.Reasons = append(plan.Reasons, fmt.Sprintf("%s: output %s modified", s, rel))
				return plan
			}
		}
		plan.Skip = append(plan.Skip, s)
	}
	return plan
}

// Bundle is a single episode directory.
type Bundle struct {
	dir string
}

// New creates (but does not touch the disk for) a bundle rooted at dir.
func New(dir string) (*Bundle, error) {
	if dir == "" {
		return nil, errors.New("bundle dir must not be empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("abs bundle dir: %w", err)
	}
	return &Bundle{dir: abs}, nil
}

// Dir is the absolute bundle root.
func (b *Bundle) Dir() string { return b.dir }

// Path resolves a bundle-relative path, rejecting escapes.
func (b *Bundle) Path(rel string) (string, error) { return fsutil.SafeJoin(b.dir, rel) }

func (b *Bundle) metaPath() string       { return filepath.Join(b.dir, "meta.json") }
func (b *Bundle) checkpointPath() string { return filepath.Join(b.dir, ".state.json") }

// Ensure creates the bundle skeleton.
func (b *Bundle) Ensure() error {
	for _, rel := range []string{"source", "audio", "text", "runs"} {
		if err := os.MkdirAll(filepath.Join(b.dir, rel), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", rel, err)
		}
	}
	return nil
}

// Exists reports whether the bundle has been initialised.
func (b *Bundle) Exists() bool {
	_, err := os.Stat(b.metaPath())
	return err == nil
}

// Meta is the persisted bundle metadata (subset of bundle-meta.schema.json that
// the skeleton actually writes today).
type Meta struct {
	Schema     int                 `json:"schema"`
	VideoID    string              `json:"video_id"`
	Title      string              `json:"title,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
	DurationMS int                 `json:"duration_ms,omitempty"`
	Source     Source              `json:"source"`
	Status     string              `json:"status"`
	Stages     map[string]StageRun `json:"stages"`
	Provenance Provenance          `json:"provenance"`
}

// Source describes where the episode came from.
type Source struct {
	Kind  string `json:"kind"`
	URL   string `json:"url,omitempty"`
	Origin string `json:"origin,omitempty"`
}

// StageRun records the result of one stage.
type StageRun struct {
	State     string `json:"state"`
	ElapsedMS int64  `json:"elapsed_ms,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"error_message,omitempty"`
}

// Provenance is the reproducibility record (G-5).
type Provenance struct {
	Engines           map[string]EngineInfo `json:"engines"`
	ParamsSnapshotHash string               `json:"params_snapshot_hash"`
}

// EngineInfo pins the exact engine/model used.
type EngineInfo struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Backend      string `json:"backend,omitempty"`
	ModelID      string `json:"model_id,omitempty"`
	ModelSHA12   string `json:"model_sha12,omitempty"`
	Quantization string `json:"quantization,omitempty"`
}

// ReadMeta loads meta.json. A missing file is reported as os.ErrNotExist so the
// caller can distinguish "new episode" from "corrupt episode".
func (b *Bundle) ReadMeta() (Meta, error) {
	raw, err := os.ReadFile(b.metaPath())
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err = json.Unmarshal(raw, &m); err != nil {
		return Meta{}, fmt.Errorf("parse meta.json: %w", err)
	}
	return m, nil
}

// WriteMeta persists meta.json atomically.
func (b *Bundle) WriteMeta(m Meta) error {
	m.Schema = 1
	m.UpdatedAt = time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = m.UpdatedAt
	}
	if m.Stages == nil {
		m.Stages = map[string]StageRun{}
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	if err = fsutil.WriteFileAtomic(b.metaPath(), append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	return nil
}

// ReadCheckpoint loads .state.json, returning an empty checkpoint when absent.
func (b *Bundle) ReadCheckpoint(videoID string) (Checkpoint, error) {
	raw, err := os.ReadFile(b.checkpointPath())
	if errors.Is(err, os.ErrNotExist) {
		return NewCheckpoint(videoID), nil
	}
	if err != nil {
		return Checkpoint{}, fmt.Errorf("read checkpoint: %w", err)
	}
	var cp Checkpoint
	if err = json.Unmarshal(raw, &cp); err != nil {
		// A corrupt checkpoint must not brick the episode: start fresh.
		return NewCheckpoint(videoID), nil
	}
	if cp.Params == nil {
		cp.Params = map[string]string{}
	}
	if cp.Files == nil {
		cp.Files = map[string]Fingerprint{}
	}
	return cp, nil
}

// WriteCheckpoint persists .state.json atomically.
func (b *Bundle) WriteCheckpoint(cp Checkpoint) error {
	cp.Schema = 1
	cp.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	if err = fsutil.WriteFileAtomic(b.checkpointPath(), append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	return nil
}

// MarkDone records a stage as completed and snapshots its outputs' fingerprints.
//
// Missing outputs are returned rather than raised as an error: a skeleton stage
// (resolve/export) legitimately produces nothing yet, and a stage that claims
// success but writes nothing must still leave a resumable checkpoint. Checkpoint
// .Plan will catch the gap on the next run because Observed() will not list it.
func (b *Bundle) MarkDone(cp *Checkpoint, stage Stage, paramsHash string) (missing []string, err error) {
	for _, rel := range stageOutputs[stage] {
		full := filepath.Join(b.dir, filepath.FromSlash(rel))
		info, statErr := os.Stat(full)
		if statErr != nil {
			missing = append(missing, rel)
			continue
		}
		cp.Files[rel] = Fingerprint{Size: info.Size(), ModTime: info.ModTime().UnixNano()}
	}
	cp.Params[string(stage)] = paramsHash

	for _, s := range cp.Completed {
		if s == stage {
			return missing, nil
		}
	}
	cp.Completed = append(cp.Completed, stage)
	return missing, nil
}

// Observe fingerprints every recorded bundle-relative path that still exists.
func (b *Bundle) Observe() map[string]Fingerprint {
	out := map[string]Fingerprint{}
	for _, rels := range stageOutputs {
		for _, rel := range rels {
			info, err := os.Stat(filepath.Join(b.dir, filepath.FromSlash(rel)))
			if err != nil {
				continue
			}
			out[rel] = Fingerprint{Size: info.Size(), ModTime: info.ModTime().UnixNano()}
		}
	}
	return out
}

// HashParams is a tiny deterministic idempotency key for a parameter set. It is
// not a security hash; it only needs to change whenever behaviour changes.
func HashParams(parts ...string) string {
	h := uint64(1469598103934665603) // FNV-1a offset basis
	for _, p := range parts {
		for i := 0; i < len(p); i++ {
			h ^= uint64(p[i])
			h *= 1099511628211
		}
		h ^= 0xff
		h *= 1099511628211
	}
	return fmt.Sprintf("fnv:%016x", h)
}
