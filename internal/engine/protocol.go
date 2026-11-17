// Package engine defines the wire protocol between the Go orchestrator and the
// out-of-process inference engines (ADR-002).
//
// The contract is one JSON object per line: stdout carries protocol frames only,
// stderr carries human logs. Unknown frame types must be ignored so a newer
// engine never breaks an older app.
package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Frame kinds emitted by an engine.
const (
	EvReady    = "ready"
	EvProgress = "progress"
	EvSegment  = "segment"
	EvLog      = "log"
	EvDone     = "done"
	EvError    = "error"
)

// Command kinds accepted by an engine.
const (
	CmdProbe      = "probe"
	CmdTranscribe = "transcribe"
	CmdSeparate   = "separate"
	CmdCancel     = "cancel"
)

// Request is one command sent to an engine on stdin.
type Request struct {
	ID     string            `json:"id"`
	Cmd    string            `json:"cmd"`
	Input  string            `json:"input,omitempty"`
	Params map[string]any    `json:"params,omitempty"`
	OutDir string            `json:"out_dir,omitempty"`
}

// Frame is one message emitted by an engine on stdout.
type Frame struct {
	ID           string   `json:"id,omitempty"`
	Ev           string   `json:"ev"`
	Engine       string   `json:"engine,omitempty"`
	Version      string   `json:"version,omitempty"`
	Backend      string   `json:"backend,omitempty"`
	Stage        string   `json:"stage,omitempty"`
	Pct          int      `json:"pct,omitempty"`
	SegmentsDone int      `json:"segments_done,omitempty"`
	Idx          int      `json:"idx,omitempty"`
	StartMS      int      `json:"start_ms,omitempty"`
	EndMS        int      `json:"end_ms,omitempty"`
	Text         string   `json:"text,omitempty"`
	Confidence   float64  `json:"confidence,omitempty"`
	Level        string   `json:"level,omitempty"`
	Msg          string   `json:"msg,omitempty"`
	OK           bool     `json:"ok,omitempty"`
	Outputs      []string `json:"outputs,omitempty"`
	ElapsedMS    int64    `json:"elapsed_ms,omitempty"`
	Code         string   `json:"code,omitempty"`
	Message      string   `json:"message,omitempty"`
	Retryable    bool     `json:"retryable,omitempty"`
}

// EncodeRequest serialises a request as a single JSONL line.
func EncodeRequest(w io.Writer, r Request) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	if _, err = w.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	return nil
}

// Reader parses engine stdout into frames.
type Reader struct {
	sc   *bufio.Scanner
	bad  int
	id   string
}

// NewReader binds a frame reader to one request id.
func NewReader(r io.Reader, requestID string) *Reader {
	sc := bufio.NewScanner(r)
	// Model paths and long log lines can exceed the default 64 KB token.
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	return &Reader{sc: sc, id: requestID}
}

// MaxBadFrames is how many unparseable lines are tolerated before giving up.
const MaxBadFrames = 10

// Next returns the next frame. It reports io.EOF when the stream ends.
//
// Malformed lines are counted and skipped rather than treated as fatal: one bad
// log line must not kill a 40-minute transcription.
func (r *Reader) Next() (Frame, error) {
	for r.sc.Scan() {
		line := strings.TrimSpace(r.sc.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue // human log leakage on stdout
		}
		var f Frame
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			r.bad++
			if r.bad > MaxBadFrames {
				return Frame{}, fmt.Errorf("engine emitted %d malformed frames", r.bad)
			}
			continue
		}
		if f.Ev == "" {
			continue // forward compatibility: ignore unknown shapes
		}
		if f.ID == "" {
			f.ID = r.id
		}
		return f, nil
	}
	if err := r.sc.Err(); err != nil {
		return Frame{}, fmt.Errorf("read engine stream: %w", err)
	}
	return Frame{}, io.EOF
}

// ErrorFrame converts an engine error frame into a Go error.
func ErrorFrame(f Frame) error {
	if f.Ev != EvError {
		return nil
	}
	return fmt.Errorf("%s: %s", f.Code, f.Message)
}

// CodeOOM is the error code the scheduler reacts to by downgrading the model.
const CodeOOM = "OOM"
