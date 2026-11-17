package engine

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestEncodeRequestIsOneLine(t *testing.T) {
	var buf bytes.Buffer
	err := EncodeRequest(&buf, Request{ID: "r1", Cmd: CmdTranscribe, Input: "a.wav",
		Params: map[string]any{"model": "medium"}})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out := buf.String()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("want exactly one trailing newline, got %q", out)
	}
	if !strings.Contains(out, `"cmd":"transcribe"`) {
		t.Errorf("payload = %q", out)
	}
}

// TestReaderParsesFramesAndSegments covers the happy path plus the forward
// compatibility rule: unknown events must be ignored, not fatal.
func TestReaderParsesFramesAndSegments(t *testing.T) {
	stream := strings.Join([]string{
		`{"id":"r1","ev":"ready","engine":"whisper.cpp","version":"1.7.4","backend":"cuda12"}`,
		`{"id":"r1","ev":"progress","stage":"decode","pct":42,"segments_done":3}`,
		`{"id":"r1","ev":"segment","idx":0,"start_ms":0,"end_ms":1200,"text":"a","confidence":0.9}`,
		`{"id":"r1","ev":"future-event","whatever":true}`,
		`human log line that leaked onto stdout`,
		`{"id":"r1","ev":"done","ok":true,"outputs":["t.json"],"elapsed_ms":1234}`,
	}, "\n")

	r := NewReader(strings.NewReader(stream), "r1")
	var kinds []string
	for {
		f, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		kinds = append(kinds, f.Ev)
		if f.Ev == EvSegment && f.ID != "r1" {
			t.Errorf("frame id = %q, want r1", f.ID)
		}
	}

	want := []string{EvReady, EvProgress, EvSegment, "future-event", EvDone}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("kinds[%d] = %q, want %q", i, kinds[i], want[i])
		}
	}
}

func TestReaderToleratesJunkThenFails(t *testing.T) {
	var b strings.Builder
	for i := 0; i < MaxBadFrames+2; i++ {
		b.WriteString(`{"broken": `)
		b.WriteString("\n")
	}
	r := NewReader(strings.NewReader(b.String()), "r1")
	_, err := r.Next()
	if err == nil {
		t.Fatal("expected failure after exceeding the malformed frame budget")
	}
	if errors.Is(err, io.EOF) {
		t.Fatal("must not report EOF for malformed input")
	}
}

func TestReaderSkipsFramesWithoutEvent(t *testing.T) {
	r := NewReader(strings.NewReader(`{"id":"r1","pct":10}`+"\n"+`{"ev":"done","ok":true}`+"\n"), "r1")
	f, err := r.Next()
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if f.Ev != EvDone {
		t.Errorf("ev = %q, want done", f.Ev)
	}
	if _, err = r.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("want EOF, got %v", err)
	}
}

func TestErrorFrame(t *testing.T) {
	if err := ErrorFrame(Frame{Ev: EvDone}); err != nil {
		t.Errorf("non-error frame produced %v", err)
	}
	err := ErrorFrame(Frame{Ev: EvError, Code: CodeOOM, Message: "cuda oom"})
	if err == nil {
		t.Fatal("error frame must convert to an error")
	}
	if !strings.Contains(err.Error(), CodeOOM) {
		t.Errorf("err = %v, want OOM code", err)
	}
}
