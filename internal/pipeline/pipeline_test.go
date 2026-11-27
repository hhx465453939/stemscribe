package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hhx465453939/stemscribe/internal/store"
)

// fakeEngine stands in for whisper.cpp / audio-separator so the orchestration
// can be verified on a machine (and in CI) with no GPU (AC-10).
type fakeEngine struct {
	segments []Segment
	writeErr error
	sepCalls int
}

func (f *fakeEngine) Transcribe(_ context.Context, _, outPath string, _ TranscribeOptions, onSeg func(Segment)) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	for _, s := range f.segments {
		onSeg(s)
	}
	return WriteTranscript(outPath, f.segments)
}

func (f *fakeEngine) Separate(_ context.Context, _, outDir string, opts SeparateOptions) (map[string]string, error) {
	f.sepCalls++
	want := opts.Stems
	if len(want) == 0 {
		want = []string{"vocals", "instrumental"}
	}
	stems := map[string]string{}
	for _, stem := range want {
		p := filepath.Join(outDir, stem+".wav")
		if err := os.WriteFile(p, []byte("RIFF...."), 0o644); err != nil {
			return nil, err
		}
		stems[stem] = p
	}
	if err := os.WriteFile(filepath.Join(outDir, "vocals16k.wav"), []byte("RIFF...."), 0o644); err != nil {
		return nil, err
	}
	return stems, nil
}

func newBundle(t *testing.T) *store.Bundle {
	t.Helper()
	b, err := store.New(filepath.Join(t.TempDir(), "ep"))
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if err = b.Ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	return b
}

// AC-10: a 30 s synthetic clip yields a transcript with >0 segments whose start
// times strictly increase.
func TestE2EProducesOrderedTranscript(t *testing.T) {
	b := newBundle(t)
	eng := &fakeEngine{segments: []Segment{
		{StartMS: 0, EndMS: 2000, Text: "第一段内容。", Confidence: 0.92},
		{StartMS: 2000, EndMS: 5000, Text: "第二段内容。", Confidence: 0.88},
		{StartMS: 5000, EndMS: 9000, Text: "第三段内容。", Confidence: 0.9},
	}}

	var events []Event
	r := &Runner{Engine: eng, Emit: func(e Event) { events = append(events, e) }}

	res, err := r.Run(context.Background(), Request{
		Bundle: b, VideoID: "test1", Separate: false,
		Transcribe: TranscribeOptions{Model: "medium"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Completed) == 0 {
		t.Fatal("no stages completed")
	}

	path, _ := b.Path("text/transcript.json")
	segs, err := ReadTranscript(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(segs) == 0 {
		t.Fatal("transcript has 0 segments")
	}
	for i := 1; i < len(segs); i++ {
		if segs[i].StartMS <= segs[i-1].StartMS {
			t.Fatalf("start times not strictly increasing at %d: %d <= %d", i, segs[i].StartMS, segs[i-1].StartMS)
		}
	}
	if len(events) == 0 {
		t.Error("no events emitted; the UI would show nothing")
	}
	if eng.sepCalls != 0 {
		t.Error("separation ran although it was disabled")
	}
}

func TestRunSkipsSeparateWhenDisabled(t *testing.T) {
	b := newBundle(t)
	eng := &fakeEngine{segments: []Segment{{StartMS: 0, EndMS: 1000, Text: "x。", Confidence: 0.9}}}
	r := &Runner{Engine: eng}
	if _, err := r.Run(context.Background(), Request{Bundle: b, VideoID: "t", Separate: false}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if eng.sepCalls != 0 {
		t.Errorf("separate called %d times, want 0", eng.sepCalls)
	}
}

func TestRunSeparatesWhenEnabled(t *testing.T) {
	b := newBundle(t)
	full, _ := b.Path("audio/full.wav")
	if err := os.WriteFile(full, []byte("RIFF...."), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	eng := &fakeEngine{segments: []Segment{{StartMS: 0, EndMS: 1000, Text: "x。", Confidence: 0.9}}}
	r := &Runner{Engine: eng}
	if _, err := r.Run(context.Background(), Request{Bundle: b, VideoID: "t", Separate: true}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if eng.sepCalls != 1 {
		t.Errorf("separate called %d times, want 1", eng.sepCalls)
	}
}

// A 4-stem model always computes every track; the retention choice decides what
// stays on disk. Unchecked stems must be gone before the checkpoint lists outputs,
// otherwise the next plan waits forever for files this run deleted on purpose.
func TestSeparateDropsUnselectedStems(t *testing.T) {
	b := newBundle(t)
	full, _ := b.Path("audio/full.wav")
	if err := os.WriteFile(full, []byte("RIFF...."), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	eng := &fakeEngine{segments: []Segment{{StartMS: 0, EndMS: 1000, Text: "x。", Confidence: 0.9}}}
	var notes []string
	r := &Runner{Engine: eng, Emit: func(e Event) {
		if e.Kind == "stems" {
			notes = append(notes, e.Note)
		}
	}}
	all := []string{"vocals", "instrumental", "drums", "bass"}
	if _, err := r.Run(context.Background(), Request{
		Bundle: b, VideoID: "t", Separate: true,
		SeparateOpts: SeparateOptions{Stems: all, Keep: []string{"vocals"}},
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, stem := range []string{"instrumental", "drums", "bass"} {
		p, _ := b.Path("audio/" + stem + ".wav")
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s.wav still on disk (err=%v); the user asked to keep vocals only", stem, err)
		}
	}
	kept, _ := b.Path("audio/vocals.wav")
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("vocals.wav must survive: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("emitted %d stem notes, want 1 (deletions must be auditable): %v", len(notes), notes)
	}
	if !strings.Contains(notes[0], "kept=vocals") || !strings.Contains(notes[0], "dropped=bass,drums,instrumental") {
		t.Errorf("stem note = %q, want kept/dropped lists", notes[0])
	}
}

func TestRunStopsOnEngineError(t *testing.T) {
	b := newBundle(t)
	eng := &fakeEngine{writeErr: context.DeadlineExceeded}
	r := &Runner{Engine: eng}
	res, err := r.Run(context.Background(), Request{Bundle: b, VideoID: "t"})
	if err == nil {
		t.Fatal("engine failure must surface as an error")
	}
	if res.FailedAt != store.StageTranscribe {
		t.Errorf("failedAt = %q, want transcribe", res.FailedAt)
	}
}

func TestRunResumesFromPlan(t *testing.T) {
	b := newBundle(t)
	eng := &fakeEngine{segments: []Segment{{StartMS: 0, EndMS: 1000, Text: "x。", Confidence: 0.9}}}

	// First pass: everything runs.
	r := &Runner{Engine: eng}
	if _, err := r.Run(context.Background(), Request{Bundle: b, VideoID: "t"}); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Second pass: transcribe+align are already valid, so they must be skipped.
	cp, err := b.ReadCheckpoint("t")
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	plan := cp.Plan(b.Observe(), nil, map[store.Stage]bool{store.StageSeparate: true, store.StagePolish: true})

	r2 := &Runner{Engine: &fakeEngine{writeErr: context.Canceled}}
	var skipped []string
	r2.Emit = func(e Event) {
		if e.Kind == "stage.skip" {
			skipped = append(skipped, e.Stage)
		}
	}
	if _, err = r2.Run(context.Background(), Request{Bundle: b, VideoID: "t", Plan: plan}); err != nil {
		t.Fatalf("resume run: %v (already-valid stages must not be re-executed)", err)
	}
	joined := strings.Join(skipped, ",")
	if !strings.Contains(joined, string(store.StageTranscribe)) {
		t.Errorf("transcribe was not skipped on resume: %v", skipped)
	}
}

// AC-04: changing a parameter must invalidate transcribe, and leaving it alone
// must not. The first half is what the mechanism exists for; the second half is
// the one that broke silently, because an empty hash on one side and a fallback on
// the other made identical settings look like a change on every run.
func TestPlanInvalidatesTranscribeWhenParamsChange(t *testing.T) {
	b := newBundle(t)
	eng := &fakeEngine{segments: []Segment{{StartMS: 0, EndMS: 1000, Text: "x。", Confidence: 0.9}}}
	warm := map[store.Stage]bool{store.StageSeparate: true, store.StagePolish: true}

	same := map[string]string{"transcribe": "beam_size=1"}
	r := &Runner{Engine: eng}
	if _, err := r.Run(context.Background(), Request{Bundle: b, VideoID: "t", Params: same}); err != nil {
		t.Fatalf("first run: %v", err)
	}

	cp, err := b.ReadCheckpoint("t")
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if got := cp.Params[string(store.StageTranscribe)]; got != "beam_size=1" {
		t.Fatalf("checkpoint recorded %q, want the caller's parameter hash; a stage-name "+
			"constant here makes every parameter change invisible", got)
	}

	if p := cp.Plan(b.Observe(), same, warm); len(p.Reasons) != 0 {
		t.Errorf("identical parameters must not invalidate anything, got %v", p.Reasons)
	}

	changed := map[string]string{"transcribe": "beam_size=8"}
	p := cp.Plan(b.Observe(), changed, warm)
	if p.ResumeAt != store.StageTranscribe {
		t.Errorf("resume at %q, want %q", p.ResumeAt, store.StageTranscribe)
	}
	if len(p.Reasons) == 0 || !strings.Contains(strings.Join(p.Reasons, ";"), "parameters changed") {
		t.Errorf("reasons %v should name the parameter change", p.Reasons)
	}
}

func TestRunRejectsNilEngine(t *testing.T) {
	b := newBundle(t)
	if _, err := (&Runner{}).Run(context.Background(), Request{Bundle: b}); err == nil {
		t.Fatal("nil engine must be rejected")
	}
}

func TestAlignRepairMergesDangling(t *testing.T) {
	segs := []Segment{
		{StartMS: 0, EndMS: 1000, Text: "他说这个方案的", Confidence: 0.9},
		{StartMS: 1000, EndMS: 2000, Text: "边界在这里。", Confidence: 0.9},
	}
	out := AlignRepair(segs)
	if len(out) != 1 {
		t.Fatalf("segments = %d, want merged into 1", len(out))
	}
	if !strings.Contains(out[0].Text, "边界在这里") {
		t.Errorf("merged text = %q", out[0].Text)
	}
	if out[0].EndMS != 2000 {
		t.Errorf("end = %d, want 2000", out[0].EndMS)
	}
}

func TestAlignRepairDropsHallucinationRun(t *testing.T) {
	segs := []Segment{
		{StartMS: 0, EndMS: 1000, Text: "谢谢观看。", Confidence: 0.9},
		{StartMS: 1000, EndMS: 2000, Text: "谢谢观看。", Confidence: 0.9},
		{StartMS: 2000, EndMS: 3000, Text: "谢谢观看。", Confidence: 0.9},
		{StartMS: 3000, EndMS: 4000, Text: "谢谢观看。", Confidence: 0.9},
		{StartMS: 4000, EndMS: 5000, Text: "真正的内容。", Confidence: 0.9},
	}
	out := AlignRepair(segs)
	if len(out) != 2 {
		t.Fatalf("segments = %d, want 2 (one kept + the real line)", len(out))
	}
	if !strings.Contains(out[1].Text, "真正的内容") {
		t.Errorf("last segment = %q", out[1].Text)
	}
}

func TestAlignRepairMarksLowConfidence(t *testing.T) {
	segs := []Segment{{StartMS: 0, EndMS: 1000, Text: "模糊的一段。", Confidence: 0.2}}
	out := AlignRepair(segs)
	if len(out) != 1 {
		t.Fatalf("segments = %d, want 1 (low confidence must not delete)", len(out))
	}
	if !out[0].LowConf {
		t.Error("low confidence segment must be marked, not removed")
	}
}

func TestAlignRepairDropsEmpty(t *testing.T) {
	segs := []Segment{{StartMS: 0, Text: "   "}, {StartMS: 1000, EndMS: 2000, Text: "ok。", Confidence: 0.9}}
	out := AlignRepair(segs)
	if len(out) != 1 || out[0].Text != "ok。" {
		t.Fatalf("out = %+v", out)
	}
}

func TestSubtitleFormats(t *testing.T) {
	segs := []Segment{{StartMS: 1000, EndMS: 2500, Text: "内容。"}}
	srt := ToSRT(segs)
	if !strings.Contains(srt, "00:00:01,000 --> 00:00:02,500") {
		t.Errorf("srt = %q", srt)
	}
	if !strings.HasPrefix(srt, "1\n") {
		t.Errorf("srt must start with index 1: %q", srt)
	}
	vtt := ToVTT(segs)
	if !strings.HasPrefix(vtt, "WEBVTT") {
		t.Errorf("vtt = %q", vtt)
	}
	if !strings.Contains(vtt, "00:00:01.000 --> 00:00:02.500") {
		t.Errorf("vtt timestamps = %q", vtt)
	}
	md := ToMarkdown("标题", segs)
	if !strings.Contains(md, "[00:00:01] 内容。") {
		t.Errorf("md = %q", md)
	}
}

func TestParseSegmentsAcceptsWrapped(t *testing.T) {
	wrapped := []byte(`{"segments":[{"start_ms":0,"end_ms":10,"text":"a","confidence":1}]}`)
	segs, err := parseSegments(wrapped)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(segs) != 1 || segs[0].Text != "a" {
		t.Fatalf("segs = %+v", segs)
	}
	direct := []byte(`[{"start_ms":0,"end_ms":10,"text":"b","confidence":1}]`)
	if segs, err = parseSegments(direct); err != nil || len(segs) != 1 {
		t.Fatalf("direct parse: %v %+v", err, segs)
	}
	if _, err = parseSegments([]byte("nonsense")); err == nil {
		t.Error("garbage must be rejected")
	}
}
