package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hhx465453939/stemscribe/internal/pipeline"
)

func TestParseProgress(t *testing.T) {
	cases := []struct {
		line string
		want int
		ok   bool
	}{
		{"whisper_print_progress_callback: progress =  10%", 10, true},
		{"whisper_print_progress_callback: progress = 100%", 100, true},
		{"progress = 7%", 7, true},
		{"whisper_model_load: loading model", 0, false},
		{"progress = abc%", 0, false},
		{"progress = 250%", 0, false},
		{"no percent here", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseProgress(c.line)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseProgress(%q) = (%d,%v), want (%d,%v)", c.line, got, ok, c.want, c.ok)
		}
	}
}

func TestParseClockMS(t *testing.T) {
	cases := map[string]int{
		"00:00:00,000": 0,
		"00:00:03,500": 3500,
		"00:01:02,250": 62250,
		"01:00:00.000": 3600000,
		"00:00:07.5":   7500,
		"12:34":        754000,
		"":             0,
		"garbage":      0,
	}
	for in, want := range cases {
		if got := parseClockMS(in); got != want {
			t.Errorf("parseClockMS(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseTranscriptOffsets(t *testing.T) {
	raw := []byte(`{"transcription":[
	  {"offsets":{"from":0,"to":3200},"timestamps":{"from":"00:00:00,000","to":"00:00:03,200"},"text":" 第一段内容。"},
	  {"offsets":{"from":3200,"to":7100},"timestamps":{"from":"00:00:03,200","to":"00:00:07,100"},"text":"第二段内容。"}
	]}`)
	segs, err := ParseTranscript(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("segments = %d, want 2", len(segs))
	}
	if segs[0].StartMS != 0 || segs[0].EndMS != 3200 {
		t.Errorf("seg0 = %d..%d", segs[0].StartMS, segs[0].EndMS)
	}
	if segs[0].Text != "第一段内容。" {
		t.Errorf("seg0 text = %q (whitespace must be trimmed)", segs[0].Text)
	}
	if segs[1].StartMS != 3200 {
		t.Errorf("seg1 start = %d", segs[1].StartMS)
	}
	// Confidence must stay unknown, otherwise alignment would flag every line.
	if segs[0].Confidence != 0 || segs[0].LowConf {
		t.Errorf("confidence = %v lowConf = %v, want unknown", segs[0].Confidence, segs[0].LowConf)
	}
}

func TestParseTimestampsFallback(t *testing.T) {
	raw := []byte(`{"segments":[{"timestamps":{"from":"00:00:01,000","to":"00:00:02,500"},"text":"x"}]}`)
	segs, err := ParseTranscript(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(segs) != 1 || segs[0].StartMS != 1000 || segs[0].EndMS != 2500 {
		t.Fatalf("segs = %+v", segs)
	}
}

func TestParseRejectsEmpty(t *testing.T) {
	if _, err := ParseTranscript([]byte(`{"transcription":[]}`)); err == nil {
		t.Error("empty transcript must be an error, not a silent empty result")
	}
	if _, err := ParseTranscript([]byte(`not json`)); err == nil {
		t.Error("garbage must be rejected")
	}
}

func TestParseSkipsBlankText(t *testing.T) {
	raw := []byte(`{"transcription":[{"offsets":{"from":0,"to":100},"text":"   "},{"offsets":{"from":100,"to":200},"text":"ok"}]}`)
	segs, err := ParseTranscript(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(segs) != 1 || segs[0].Text != "ok" {
		t.Fatalf("segs = %+v", segs)
	}
}

func TestClassifyRunError(t *testing.T) {
	cases := []struct {
		stderr string
		code   int
		want   string
	}{
		{"CUDA error: out of memory", 1, CodeOOM},
		{"failed to allocate Vulkan buffer", 1, CodeOOM},
		{"error: failed to open 'x.wav'", 1, "ENGINE_MISSING_INPUT"},
		{"failed to load model", 1, "ENGINE_BAD_MODEL"},
		{"something else", 7, "ENGINE_EXIT_7"},
	}
	for _, c := range cases {
		if got := ClassifyRunError(c.stderr, c.code); got != c.want {
			t.Errorf("ClassifyRunError(%q) = %q, want %q", c.stderr, got, c.want)
		}
	}
}

// TestWhisperWithFakeExec verifies the adapter wiring end to end without the real
// binary: flags, progress forwarding, JSON pickup, and our output contract.
func TestWhisperWithFakeExec(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "ggml-tiny.bin")
	audio := filepath.Join(dir, "a.wav")
	for _, p := range []string{model, audio} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}
	out := filepath.Join(dir, "text", "transcript.json")

	var gotArgs []string
	w := &Whisper{
		Exe:       filepath.Join(dir, "whisper-cli.exe"),
		ModelPath: model,
		Backend:   "cpu",
		Version:   "test",
		exec: func(_ context.Context, _ string, args []string, onLine func(string)) ([]byte, string, error) {
			gotArgs = args
			if onLine != nil {
				onLine("whisper_print_progress_callback: progress =  50%")
				onLine("whisper_print_progress_callback: progress = 100%")
			}
			prefix := ""
			for i, a := range args {
				if a == "-of" && i+1 < len(args) {
					prefix = args[i+1]
				}
			}
			payload := `{"transcription":[{"offsets":{"from":0,"to":1200},"text":"hello"}]}`
			return nil, "", os.WriteFile(prefix+".json", []byte(payload), 0o644)
		},
	}

	var segs []pipeline.Segment
	err := w.Transcribe(context.Background(), audio, out, pipeline.TranscribeOptions{Language: "en"}, func(s pipeline.Segment) {
		segs = append(segs, s)
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if len(segs) != 1 || segs[0].Text != "hello" {
		t.Fatalf("callback segments = %+v", segs)
	}

	// -ng must be present for cpu builds: otherwise a mis-built runtime could
	// silently fall back to CPU and we would never know.
	if !hasArg(gotArgs, "-ng") {
		t.Errorf("cpu backend must pass -ng, args = %v", gotArgs)
	}
	if !hasArg(gotArgs, "-oj") || !hasArg(gotArgs, "-of") {
		t.Errorf("json output flags missing, args = %v", gotArgs)
	}
	if !hasArg(gotArgs, "-l") {
		t.Errorf("language flag missing, args = %v", gotArgs)
	}

	// The adapter must write our contract, not whisper's json.
	data, readErr := os.ReadFile(out)
	if readErr != nil {
		t.Fatalf("read transcript: %v", readErr)
	}
	if !hasBytes(data, []byte(`"start_ms"`)) {
		t.Errorf("output is not our segment contract: %s", string(data))
	}
}

func TestWhisperSeparateUnimplemented(t *testing.T) {
	w := &Whisper{}
	if err := w.Separate(context.Background(), "a", "b", "c", pipeline.SeparateOptions{}); err == nil {
		t.Fatal("separation must fail loudly until CHG-0003 lands")
	}
}

func TestWhisperRejectsMissingFiles(t *testing.T) {
	w := &Whisper{ModelPath: "/nope/model.bin"}
	if err := w.Transcribe(context.Background(), "/nope/a.wav", "/tmp/o.json", pipeline.TranscribeOptions{}, nil); err == nil {
		t.Error("missing model must be rejected")
	}
}

func TestFFmpegProbeDuration(t *testing.T) {
	f := &FFmpeg{Exe: "ffmpeg"}
	f.run = func(_ context.Context, _ string, _ []string) (string, string, error) {
		return "", "  Duration: 00:00:30.05, start: 0.000000, bitrate: 128 kb/s", nil
	}
	d, err := f.ProbeDuration(context.Background(), os.Args[0])
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if d.Seconds() != 30.05 {
		t.Errorf("duration = %v, want 30.05s", d)
	}
}

func TestFFmpegProbeRejectsBad(t *testing.T) {
	f := &FFmpeg{Exe: "ffmpeg"}
	if _, err := f.ProbeDuration(context.Background(), "/nope/x.mp4"); err == nil {
		t.Error("missing input must be rejected")
	}
	f.run = func(_ context.Context, _ string, _ []string) (string, string, error) {
		return "", "no banner here", nil
	}
	if _, err := f.ProbeDuration(context.Background(), os.Args[0]); err == nil {
		t.Error("unparseable banner must be rejected")
	}
}

func TestFFmpegDemuxArgs(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out := filepath.Join(dir, "cache", "audio16k.wav")

	var gotArgs []string
	f := &FFmpeg{Exe: "ffmpeg"}
	f.run = func(_ context.Context, _ string, args []string) (string, string, error) {
		gotArgs = args
		return "", "", os.WriteFile(out, []byte("RIFF"), 0o644)
	}
	if err := f.ToWhisperWAV(context.Background(), in, out); err != nil {
		t.Fatalf("demux: %v", err)
	}
	for _, want := range []string{"-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", "-vn"} {
		if !hasArg(gotArgs, want) {
			t.Errorf("demux must pass %q (whisper only accepts 16k mono s16), args=%v", want, gotArgs)
		}
	}
}

func TestFFmpegDemuxRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f := &FFmpeg{Exe: "ffmpeg"}
	f.run = func(_ context.Context, _ string, _ []string) (string, string, error) {
		return "", "boom", nil // succeeded but wrote nothing
	}
	if err := f.ToWhisperWAV(context.Background(), in, filepath.Join(dir, "o.wav")); err == nil {
		t.Error("empty output must be an error")
	}
}

// TestDefaultExecCancelKillsChild covers AC-08 against a real child process:
// canceling the context must end it promptly instead of leaving an orphan that
// keeps a GPU allocation after the CLI is gone.
func TestDefaultExecCancelKillsChild(t *testing.T) {
	if _, err := exec.LookPath("ping"); err != nil {
		t.Skip("ping not available on this platform")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	started := time.Now()
	// ping -n 30 would run for ~29s if the context kill did nothing.
	_, _, err := defaultExec(ctx, "ping", []string{"-n", "30", "127.0.0.1"}, nil)
	elapsed := time.Since(started)

	if err == nil {
		t.Error("a canceled child must surface an error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("cancel took %v, want <= 2s (child likely orphaned)", elapsed)
	}
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func hasBytes(hay, needle []byte) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
