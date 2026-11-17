package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanResumesAtFirstIncomplete(t *testing.T) {
	cp := NewCheckpoint("v1")
	cp.Completed = []Stage{StageResolve, StageDownload}
	plan := cp.Plan(map[string]Fingerprint{}, map[string]string{}, nil)
	if plan.ResumeAt != StageProbe {
		t.Fatalf("resume = %q, want probe", plan.ResumeAt)
	}
	if len(plan.Reasons) == 0 {
		t.Error("reasons must explain the decision")
	}
	if len(plan.Skip) != 2 {
		t.Errorf("skip = %v, want the two completed stages", plan.Skip)
	}
}

func TestPlanInvalidatesOnParamChange(t *testing.T) {
	cp := NewCheckpoint("v1")
	cp.Completed = []Stage{StageResolve, StageDownload, StageProbe, StageDemux}
	cp.Params[string(StageDemux)] = "fnv:old"

	observed := map[string]Fingerprint{
		"audio/full.wav": {Size: 10, ModTime: 1},
		"audio/16k.wav":  {Size: 10, ModTime: 1},
	}
	cp.Files = observed

	plan := cp.Plan(observed, map[string]string{string(StageDemux): "fnv:new"}, nil)
	if plan.ResumeAt != StageDemux {
		t.Fatalf("resume = %q, want demux (params changed)", plan.ResumeAt)
	}
}

func TestPlanInvalidatesOnModifiedOutput(t *testing.T) {
	cp := NewCheckpoint("v1")
	cp.Completed = []Stage{StageResolve, StageDownload, StageProbe, StageDemux, StageSeparate}
	recorded := map[string]Fingerprint{
		"audio/full.wav":  {Size: 10, ModTime: 1},
		"audio/16k.wav":   {Size: 10, ModTime: 1},
		"audio/vocals.wav": {Size: 20, ModTime: 1},
	}
	cp.Files = recorded

	// Someone replaced vocals.wav by hand.
	observed := map[string]Fingerprint{
		"audio/full.wav":  {Size: 10, ModTime: 1},
		"audio/16k.wav":   {Size: 10, ModTime: 1},
		"audio/vocals.wav": {Size: 999, ModTime: 2},
	}
	plan := cp.Plan(observed, nil, nil)
	if plan.ResumeAt != StageSeparate {
		t.Fatalf("resume = %q, want separate (output modified)", plan.ResumeAt)
	}
}

func TestPlanInvalidatesOnMissingOutput(t *testing.T) {
	cp := NewCheckpoint("v1")
	cp.Completed = []Stage{StageResolve, StageDownload, StageProbe, StageDemux}
	cp.Files = map[string]Fingerprint{
		"audio/full.wav": {Size: 10, ModTime: 1},
		"audio/16k.wav":  {Size: 10, ModTime: 1},
	}
	// User deleted the 16k file.
	observed := map[string]Fingerprint{"audio/full.wav": {Size: 10, ModTime: 1}}

	plan := cp.Plan(observed, nil, nil)
	if plan.ResumeAt != StageDemux {
		t.Fatalf("resume = %q, want demux (missing output)", plan.ResumeAt)
	}
}

func TestPlanAllDone(t *testing.T) {
	cp := NewCheckpoint("v1")
	cp.Completed = append([]Stage{}, StageOrder...)
	cp.Files = map[string]Fingerprint{}
	observed := map[string]Fingerprint{}
	for _, s := range StageOrder {
		for _, rel := range Outputs(s) {
			observed[rel] = Fingerprint{Size: 1, ModTime: 1}
			cp.Files[rel] = Fingerprint{Size: 1, ModTime: 1}
		}
	}
	plan := cp.Plan(observed, nil, nil)
	if plan.ResumeAt != "" {
		t.Fatalf("resume = %q, want empty (nothing left)", plan.ResumeAt)
	}
}

func TestBundleRoundTripAndCheckpoint(t *testing.T) {
	b, err := New(filepath.Join(t.TempDir(), "ep1"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err = b.Ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	meta := Meta{VideoID: "BV1", Title: "t", Status: "running", Source: Source{Kind: "url"}}
	if err = b.WriteMeta(meta); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	got, err := b.ReadMeta()
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	if got.VideoID != "BV1" || got.Status != "running" {
		t.Errorf("meta = %+v", got)
	}

	cp := NewCheckpoint("BV1")
	// Create a real output so MarkDone fingerprints something.
	out, _ := b.Path("text/transcript.json")
	if err = os.WriteFile(out, []byte("[]"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	missing, err := b.MarkDone(&cp, StageTranscribe, "fnv:1")
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
	if err = b.WriteCheckpoint(cp); err != nil {
		t.Fatalf("write cp: %v", err)
	}
	read, err := b.ReadCheckpoint("BV1")
	if err != nil {
		t.Fatalf("read cp: %v", err)
	}
	if len(read.Completed) != 1 || read.Completed[0] != StageTranscribe {
		t.Errorf("completed = %v", read.Completed)
	}
}

func TestMarkDoneReportsMissing(t *testing.T) {
	b, _ := New(filepath.Join(t.TempDir(), "ep2"))
	if err := b.Ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	cp := NewCheckpoint("x")
	missing, err := b.MarkDone(&cp, StageTranscribe, "h")
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if len(missing) == 0 {
		t.Error("missing output must be reported, not silently accepted")
	}
}

func TestBundlePathRejectsEscape(t *testing.T) {
	b, _ := New(filepath.Join(t.TempDir(), "ep3"))
	if _, err := b.Path("../outside.txt"); err == nil {
		t.Fatal("traversal must be rejected")
	}
}

func TestReadCheckpointCorruptStartsFresh(t *testing.T) {
	b, _ := New(filepath.Join(t.TempDir(), "ep4"))
	if err := b.Ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := os.WriteFile(filepath.Join(b.Dir(), ".state.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cp, err := b.ReadCheckpoint("v")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(cp.Completed) != 0 {
		t.Error("corrupt checkpoint must not brick the episode")
	}
}

func TestHashParamsIsStableAndSensitive(t *testing.T) {
	a := HashParams("medium", "15", "cuda12")
	b := HashParams("medium", "15", "cuda12")
	c := HashParams("large", "15", "cuda12")
	if a != b {
		t.Error("same inputs must produce the same hash")
	}
	if a == c {
		t.Error("different inputs must produce a different hash")
	}
}

func TestNewRejectsEmptyDir(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("empty dir must be rejected")
	}
}
