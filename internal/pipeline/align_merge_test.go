package pipeline

import (
	"strings"
	"testing"
)

// A 30-minute Chinese episode produced lines up to 328 runes because AlignRepair
// appended dangling fragments with no ceiling. These tests pin the ceiling and the two
// invariants that must survive it: no text is lost, and the covered time range is
// unchanged.
func TestAlignCapsMergedLength(t *testing.T) {
	const frag = "这是一段口语" // 6 runes, deliberately below DanglingMaxRunes

	segs := make([]Segment, 0, 10)
	for i := 0; i < 10; i++ {
		segs = append(segs, Segment{
			StartMS:    i * 1000,
			EndMS:      (i + 1) * 1000,
			Text:       frag,
			Confidence: 0.9,
		})
	}

	out := AlignRepair(segs)
	if len(out) == 0 {
		t.Fatal("AlignRepair produced no segments")
	}
	for i, s := range out {
		if n := len([]rune(s.Text)); n > MaxMergedRunes {
			t.Errorf("segment %d has %d runes, want <= %d: %q", i, n, MaxMergedRunes, s.Text)
		}
	}
	if got, want := strings.Join(segTexts(out), ""), strings.Repeat(frag, 10); got != want {
		t.Errorf("text not preserved: got %q want %q", got, want)
	}
	if out[0].StartMS != 0 {
		t.Errorf("first StartMS = %d, want 0", out[0].StartMS)
	}
	if last := out[len(out)-1]; last.EndMS != 10000 {
		t.Errorf("last EndMS = %d, want 10000", last.EndMS)
	}
}

// A sentence that already ended must not absorb the next one, regardless of the cap.
func TestAlignNoCrossSentence(t *testing.T) {
	for _, term := range []string{"。", "！", "？", ".", "!", "?"} {
		segs := []Segment{
			{StartMS: 0, EndMS: 1000, Text: "第一句" + term, Confidence: 0.9},
			{StartMS: 1000, EndMS: 2000, Text: "第二句", Confidence: 0.9},
		}
		out := AlignRepair(segs)
		if len(out) != 2 {
			t.Errorf("%q: got %d segments, want 2 (must not merge across a sentence end)", term, len(out))
			continue
		}
		if out[0].EndMS != 1000 {
			t.Errorf("%q: first EndMS = %d, want 1000 (must not stretch)", term, out[0].EndMS)
		}
	}
}

// One genuinely long sentence may exceed the cap; what must not happen is the next
// fragment being glued onto it.
func TestAlignStopsGrowing(t *testing.T) {
	long := strings.Repeat("很", MaxMergedRunes+100)
	segs := []Segment{
		{StartMS: 0, EndMS: 1000, Text: long, Confidence: 0.9},
		{StartMS: 1000, EndMS: 2000, Text: "追加", Confidence: 0.9},
	}
	out := AlignRepair(segs)
	if len(out) == 0 {
		t.Fatal("AlignRepair produced no segments")
	}
	if strings.Contains(out[0].Text, "追加") {
		t.Errorf("an overlong segment must not absorb the next fragment: %d runes",
			len([]rune(out[0].Text)))
	}
}

func segTexts(segs []Segment) []string {
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		out = append(out, s.Text)
	}
	return out
}
