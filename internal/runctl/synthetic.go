package runctl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/pipeline"
)

// SyntheticEngine produces deterministic segments so the orchestration,
// checkpointing and artifact writing can be verified on any machine.
//
// It is NOT an inference engine: it never reads audio. The smoke test depends on
// it, and the name is kept blunt so a green smoke test is never mistaken for
// "transcription works".
type SyntheticEngine struct {
	durationMS int
}

// NewSyntheticEngine builds an engine that emits one segment per ~8 seconds.
func NewSyntheticEngine(durationMS int) *SyntheticEngine {
	if durationMS <= 0 {
		durationMS = 30000
	}
	return &SyntheticEngine{durationMS: durationMS}
}

// Transcribe emits evenly spaced segments covering the synthetic duration.
func (s *SyntheticEngine) Transcribe(_ context.Context, _, outPath string, opts pipeline.TranscribeOptions, onSeg func(pipeline.Segment)) error {
	const stepMS = 8000
	segs := make([]pipeline.Segment, 0, s.durationMS/stepMS+1)
	for start := 0; start < s.durationMS; start += stepMS {
		end := start + stepMS
		if end > s.durationMS {
			end = s.durationMS
		}
		seg := pipeline.Segment{
			StartMS:    start,
			EndMS:      end,
			Text:       fmt.Sprintf("合成语音片段 %d（模型 %s）。", start/stepMS+1, fallback(opts.Model, "medium")),
			Confidence: 0.93,
		}
		segs = append(segs, seg)
		onSeg(seg)
	}
	if err := pipeline.WriteTranscript(outPath, segs); err != nil {
		return fmt.Errorf("synthetic transcribe: %w", err)
	}
	if len(segs) == 0 {
		return fmt.Errorf("synthetic transcribe produced no segments")
	}
	return nil
}

// Separate writes placeholder stems so downstream stage checks have files to see.
func (s *SyntheticEngine) Separate(_ context.Context, _, vocalsPath, instrumentalPath string, _ pipeline.SeparateOptions) error {
	payload := []byte("RIFF....WAVEfmt synthetic placeholder")
	for _, p := range []string{vocalsPath, instrumentalPath} {
		if err := writeFile(p, payload); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
