package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/pipeline"
)

// ParseProgress extracts a percentage from one whisper-cli stderr line.
//
// Why parse text at all: whisper.cpp has no structured progress channel. The
// adapter owns this fragility (ADR-015) so the pipeline never has to know what
// the upstream CLI's stderr looks like. Version is pinned and covered by a golden
// test for exactly this reason.
func ParseProgress(line string) (int, bool) {
	const marker = "progress"
	if !strings.Contains(strings.ToLower(line), marker) {
		return 0, false
	}
	idx := strings.Index(line, "%")
	if idx <= 0 {
		return 0, false
	}
	// Walk left from '%' over digits to get the number.
	start := idx
	for start > 0 && line[start-1] >= '0' && line[start-1] <= '9' {
		start--
	}
	if start == idx {
		return 0, false
	}
	n, err := strconv.Atoi(line[start:idx])
	if err != nil || n < 0 || n > 100 {
		return 0, false
	}
	return n, true
}

// whisperJSON is the shape produced by `whisper-cli -oj`.
//
// Two layouts are accepted because the field names have moved across versions:
// offsets (milliseconds, preferred) and timestamps (clock strings, fallback).
type whisperJSON struct {
	Transcription []whisperSeg `json:"transcription"`
	Segments      []whisperSeg `json:"segments"`
}

type whisperSeg struct {
	Timestamps struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"timestamps"`
	Offsets struct {
		From int `json:"from"`
		To   int `json:"to"`
	} `json:"offsets"`
	Text string `json:"text"`
}

// ParseTranscript converts whisper-cli JSON into our segment contract.
//
// Confidence is deliberately left at zero: whisper.cpp does not emit per-segment
// confidence in this output. Zero means "unknown" downstream, which keeps the
// alignment stage from flagging every line as low confidence.
func ParseTranscript(raw []byte) ([]pipeline.Segment, error) {
	var doc whisperJSON
	if err := unmarshalJSON(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse whisper json: %w", err)
	}
	items := doc.Transcription
	if len(items) == 0 {
		items = doc.Segments
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("whisper json contains no segments")
	}

	out := make([]pipeline.Segment, 0, len(items))
	for _, it := range items {
		start, end := it.Offsets.From, it.Offsets.To
		if start == 0 && end == 0 {
			// Fall back to clock strings when offsets are absent.
			start = parseClockMS(it.Timestamps.From)
			end = parseClockMS(it.Timestamps.To)
		}
		text := strings.TrimSpace(it.Text)
		if text == "" {
			continue
		}
		if end < start {
			end = start
		}
		out = append(out, pipeline.Segment{
			StartMS: start,
			EndMS:   end,
			Text:    text,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("whisper json produced no usable segments")
	}
	return out, nil
}

// parseClockMS parses "HH:MM:SS,mmm" or "HH:MM:SS.mmm" (and the short MM:SS form).
func parseClockMS(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ",", ".")

	ms := 0
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		frac := s[dot+1:]
		s = s[:dot]
		for len(frac) < 3 {
			frac += "0"
		}
		if v, err := strconv.Atoi(frac[:3]); err == nil {
			ms = v
		}
	}
	parts := strings.Split(s, ":")
	total := 0
	for _, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return 0
		}
		total = total*60 + v
	}
	return total*1000 + ms
}

// ClassifyRunError maps upstream stderr onto our stable error codes so the
// scheduler can decide whether downgrading would help (ARCHITECTURE §7).
func ClassifyRunError(stderr string, exitCode int) string {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "out of memory") || strings.Contains(s, "cuda error") ||
		strings.Contains(s, "failed to allocate"):
		return CodeOOM
	case strings.Contains(s, "no such file") || strings.Contains(s, "failed to open"):
		return "ENGINE_MISSING_INPUT"
	case strings.Contains(s, "invalid model") || strings.Contains(s, "failed to load model"):
		return "ENGINE_BAD_MODEL"
	default:
		return fmt.Sprintf("ENGINE_EXIT_%d", exitCode)
	}
}
