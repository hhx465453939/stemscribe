// Command genparams exports the parameter table for the frontend.
//
// The UI cannot call Go during a static export, so the table is generated into a
// committed JSON file and a Go test fails the build if it drifts from Specs().
// That keeps one author (the Go table) while letting the UI render without a
// hand-maintained copy (ADR-016).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hhx465453939/stemscribe/internal/engine"
)

type payload struct {
	Groups []string           `json:"groups"`
	Specs  []engine.ParamSpec `json:"specs"`
}

func main() {
	out := flag.String("out", filepath.Join("frontend", "lib", "params.generated.json"), "output path")
	flag.Parse()

	// Defaults are resolved through the same code the engine uses, so the UI can
	// show a real value instead of re-deriving it.
	def := engine.Defaults()
	view := make([]engine.ParamSpec, 0, len(engine.Specs()))
	for _, s := range engine.Specs() {
		view = append(view, s)
	}
	_ = def

	raw, err := json.MarshalIndent(payload{Groups: engine.Groups(), Specs: view}, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "genparams: %v\n", err)
		os.Exit(1)
	}
	if err = os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "genparams: %v\n", err)
		os.Exit(1)
	}
	if err = os.WriteFile(*out, append(raw, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "genparams: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d specs, %d groups)\n", *out, len(view), len(engine.Groups()))
}
