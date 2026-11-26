package pipeline

import "testing"

func TestToTextOneLinePerSegment(t *testing.T) {
	segs := []Segment{
		{StartMS: 0, EndMS: 1000, Text: " 第一行 "},
		{StartMS: 1000, EndMS: 2000, Text: ""},
		{StartMS: 2000, EndMS: 3000, Text: "第二行"},
	}
	got := ToText(segs)
	want := "第一行\n第二行\n"
	if got != want {
		t.Fatalf("ToText = %q want %q", got, want)
	}
}
