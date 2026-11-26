package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/pipeline"
)

func unmarshalJSON(raw []byte, dst any) error { return json.Unmarshal(raw, dst) }

// ErrNotImplemented is returned by stages that this CHG deliberately delegates
// to another one (ADR-014: separation moved to CHG-0003).
var ErrNotImplemented = errors.New("not implemented in this change")

// ExecFunc runs a command and forwards each stderr line to onLine.
//
// Injected so the adapter can be tested without the real binary; the default
// implementation is the only place that touches os/exec.
type ExecFunc func(ctx context.Context, exe string, args []string, onLine func(string)) (stdout []byte, stderr string, err error)

// Whisper adapts whisper-cli to the pipeline Engine port (ADR-015).
type Whisper struct {
	// Exe is the absolute path to whisper-cli.exe of the selected runtime.
	Exe string
	// ModelPath is the absolute path to the .bin model.
	ModelPath string
	// Backend is one of cuda12 / vulkan / cpu. Only used for logging and for
	// deciding whether to pass the "no gpu" switch.
	Backend string
	// Version is the runtime version, recorded in provenance.
	Version string
	// Log receives human-readable engine output; nil discards it.
	Log *os.File

	// Params carries every user-facing option. A zero value means "the engine's own
	// defaults"; Transcribe resolves it through Defaults() so an unset struct can
	// never silently switch something off.
	Params WhisperParams

	exec ExecFunc
}

func (w *Whisper) runner() ExecFunc {
	if w.exec != nil {
		return w.exec
	}
	return defaultExec
}

// defaultExec is the real process runner.
//
// The child is killed through the context, which is what makes cancel work: the
// pipeline cancels the context and whisper-cli disappears within ~2s rather than
// holding VRAM as an orphan (AC-08).
func defaultExec(ctx context.Context, exe string, args []string, onLine func(string)) ([]byte, string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	wireCancel(cmd)

	var stdout strings.Builder
	cmd.Stdout = &stdout

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, "", fmt.Errorf("stderr pipe: %w", err)
	}
	if err = cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start %s: %w", filepath.Base(exe), err)
	}

	var stderr strings.Builder
	buf := make([]byte, 4096)
	var pending strings.Builder
	for {
		n, readErr := stderrPipe.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			stderr.WriteString(chunk)
			pending.WriteString(chunk)
			for {
				line, rest, found := strings.Cut(pending.String(), "\n")
				if !found {
					pending.Reset()
					pending.WriteString(rest)
					break
				}
				if onLine != nil {
					onLine(line)
				}
				pending.Reset()
				pending.WriteString(rest)
			}
		}
		if readErr != nil {
			break
		}
	}
	if pending.Len() > 0 && onLine != nil {
		onLine(pending.String())
	}

	waitErr := cmd.Wait()
	return []byte(stdout.String()), stderr.String(), waitErr
}

// Probe verifies the runtime is executable and reports what it is.
func (w *Whisper) Probe(ctx context.Context) (string, error) {
	if w.Exe == "" {
		return "", fmt.Errorf("runtime has no entrypoint")
	}
	if _, err := os.Stat(w.Exe); err != nil {
		return "", fmt.Errorf("runtime missing: %w", err)
	}
	stdout, stderr, err := w.runner()(ctx, w.Exe, []string{"--help"}, nil)
	if err != nil {
		return "", fmt.Errorf("probe %s: %w (%s)", filepath.Base(w.Exe), err, firstLine(stderr))
	}
	// whisper-cli prints usage on stderr in some builds; accept either stream.
	return firstLine(string(stdout) + "\n" + stderr), nil
}

// Transcribe implements pipeline.Engine.
func (w *Whisper) Transcribe(ctx context.Context, audioPath, outPath string, opts pipeline.TranscribeOptions, onSeg func(pipeline.Segment)) error {
	if w.ModelPath == "" {
		return fmt.Errorf("no model selected")
	}
	if _, err := os.Stat(w.ModelPath); err != nil {
		return fmt.Errorf("model missing: %w", err)
	}
	if _, err := os.Stat(audioPath); err != nil {
		return fmt.Errorf("audio missing: %w", err)
	}

	outDir := filepath.Dir(outPath)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create transcript dir: %w", err)
	}
	// whisper-cli writes <prefix>.json next to the prefix we give it.
	prefix := strings.TrimSuffix(outPath, filepath.Ext(outPath))

	// Base flags the adapter itself depends on.
	args := []string{
		"-m", w.ModelPath,
		"-f", audioPath,
		"-of", prefix,
		"-pp", // print progress to stderr, which we translate
	}

	// Everything else comes from the parameter table, so UI, CLI and docs render
	// from one source (ADR-016).
	p := w.Params
	paramsUnset := p == (WhisperParams{})
	if paramsUnset {
		// Zero value must mean "engine defaults", not "all features off": with the
		// raw zero struct, flash attention would read as disabled and quietly cost
		// performance on every run.
		p = Defaults()
	}
	// Legacy fields win when Params was not populated. The order matters: filling
	// defaults first would leave InputLanguage="en" in place and drop the caller's
	// choice, so "zh" silently became English.
	if opts.Language != "" && (paramsUnset || p.InputLanguage == "") {
		p.InputLanguage = opts.Language
	}
	if opts.Threads > 0 && (paramsUnset || p.Threads == 0) {
		p.Threads = opts.Threads
	}
	if opts.VAD {
		p.VAD = true
	}
	// On CPU builds there is no GPU to use; say so explicitly instead of letting
	// the binary decide, otherwise a mis-built runtime silently runs on CPU.
	if w.Backend == "cpu" {
		p.NoGPU = true
	}
	args = append(args, p.Args()...)

	lastPct := -1
	_, stderr, err := w.runner()(ctx, w.Exe, args, func(line string) {
		if w.Log != nil {
			_, _ = w.Log.WriteString("engine| " + line + "\n")
		}
		if pct, ok := ParseProgress(line); ok && pct != lastPct {
			lastPct = pct
		}
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("transcribe canceled: %w", ctxErr)
		}
		// Include the engine's own last words. Without them a failure such as
		// "exit status 10" is an opaque code with nothing to search for, which is
		// exactly how this went undiagnosed for a whole round.
		detail := lastLine(stderr)
		if detail == "" {
			detail = "engine printed nothing to stderr"
		}
		return fmt.Errorf("%s: %w: %s", ClassifyRunError(stderr, exitCodeOf(err)), err, detail)
	}

	raw, err := os.ReadFile(prefix + ".json")
	if err != nil {
		return fmt.Errorf("whisper produced no json (expected %s.json): %w", filepath.Base(prefix), err)
	}
	segs, err := ParseTranscript(raw)
	if err != nil {
		return err
	}
	for _, s := range segs {
		if onSeg != nil {
			onSeg(s)
		}
	}
	return pipeline.WriteTranscript(outPath, segs)
}

// Separate is intentionally unimplemented; separation moved to CHG-0003
// (ADR-014). Returning a typed error keeps the pipeline honest instead of
// pretending a stage ran.
func (w *Whisper) Separate(_ context.Context, _, _ string, _ pipeline.SeparateOptions) (map[string]string, error) {
	return nil, fmt.Errorf("separation: %w (moved to CHG-0003)", ErrNotImplemented)
}

func exitCodeOf(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
