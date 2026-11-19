package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestGeneratedParamsUpToDate is AC-01 enforcement: the frontend JSON must match
// the Go spec table.
//
// Without this the UI would keep rendering a stale copy after a parameter is
// added, which is the failure mode the generated file exists to prevent.
func TestGeneratedParamsUpToDate(t *testing.T) {
	path := filepath.Join("..", "..", "frontend", "lib", "params.generated.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run: go run ./cmd/genparams)", path, err)
	}

	var got struct {
		Groups []string    `json:"groups"`
		Specs  []ParamSpec `json:"specs"`
	}
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("parse generated params: %v", err)
	}

	want := Specs()
	if len(got.Specs) != len(want) {
		t.Fatalf("generated specs = %d, want %d (run: go run ./cmd/genparams)", len(got.Specs), len(want))
	}
	for i := range want {
		if got.Specs[i].Key != want[i].Key || got.Specs[i].Flag != want[i].Flag {
			t.Errorf("spec %d drifted: generated %q/%q, want %q/%q — run: go run ./cmd/genparams",
				i, got.Specs[i].Key, got.Specs[i].Flag, want[i].Key, want[i].Flag)
		}
	}

	groups := Groups()
	if len(got.Groups) != len(groups) {
		t.Fatalf("generated groups = %d, want %d", len(got.Groups), len(groups))
	}
	for i := range groups {
		if got.Groups[i] != groups[i] {
			t.Errorf("group %d drifted: %q, want %q", i, got.Groups[i], groups[i])
		}
	}
}
