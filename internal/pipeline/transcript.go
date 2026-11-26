package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseSegments decodes a transcript JSON file.
//
// Two shapes are accepted because the on-disk contract (an array) is what
// whisper.cpp dumps, while some adapters wrap it in {"segments":[...]}.
func parseSegments(raw []byte) ([]Segment, error) {
	var direct []Segment
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}
	var wrapped struct {
		Segments []Segment `json:"segments"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("parse transcript: %w", err)
	}
	return wrapped.Segments, nil
}

// marshalSegments encodes segments in the canonical array form.
func marshalSegments(segs []Segment) ([]byte, error) {
	if segs == nil {
		segs = []Segment{}
	}
	raw, err := json.MarshalIndent(segs, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal transcript: %w", err)
	}
	return append(raw, '\n'), nil
}

// ToSRT renders segments as SubRip subtitles.
func ToSRT(segs []Segment) string {
	out := ""
	for i, s := range segs {
		out += fmt.Sprintf("%d\n%s --> %s\n%s\n\n", i+1, srtClock(s.StartMS), srtClock(s.EndMS), s.Text)
	}
	return out
}

// ToVTT renders segments as WebVTT.
func ToVTT(segs []Segment) string {
	out := "WEBVTT\n\n"
	for _, s := range segs {
		out += fmt.Sprintf("%s --> %s\n%s\n\n", vttClock(s.StartMS), vttClock(s.EndMS), s.Text)
	}
	return out
}

// ToMarkdown renders anchored, human-readable transcript lines.
func ToMarkdown(title string, segs []Segment) string {
	out := "# " + title + "\n\n"
	for _, s := range segs {
		out += fmt.Sprintf("[%s] %s\n\n", clock(s.StartMS), s.Text)
	}
	return out
}

// ToText renders the plain transcript: one segment per line, no timecodes.
// This is the file most people actually read (AC-04); srt/vtt/md keep the
// timing for subtitles and review.
func ToText(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		line := strings.TrimSpace(s.Text)
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func clock(ms int) string {
	if ms < 0 {
		ms = 0
	}
	t := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d", t/3600, (t%3600)/60, t%60)
}

func srtClock(ms int) string {
	if ms < 0 {
		ms = 0
	}
	t := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", t/3600, (t%3600)/60, t%60, ms%1000)
}

func vttClock(ms int) string {
	if ms < 0 {
		ms = 0
	}
	t := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", t/3600, (t%3600)/60, t%60, ms%1000)
}
