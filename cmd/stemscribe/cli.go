package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/engine"
	"github.com/hhx465453939/stemscribe/internal/pipeline"
	"github.com/hhx465453939/stemscribe/internal/resource"
	"github.com/hhx465453939/stemscribe/internal/runctl"
	"github.com/hhx465453939/stemscribe/internal/store"
)

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	input := fs.String("input", "", "path to a local media file")
	workdir := fs.String("workdir", "", "work directory (default: workdir next to the executable)")
	title := fs.String("title", "", "display title for the bundle")
	noSeparate := fs.Bool("no-separate", false, "skip vocal separation")
	noPolish := fs.Bool("no-polish", false, "skip LLM polishing (default: already skipped)")
	durationMS := fs.Int("duration-ms", 30000, "synthetic duration used by the smoke engine")
	engineName := fs.String("engine", "auto", "engine: auto (real whisper.cpp) | synthetic (test double)")
	modelFlag := fs.String("model", "", "model id from the registry (default: from settings)")
	sepFlag := fs.String("separator", "", "separation model id from the registry (default: from settings)")
	// Separation knobs, in the units of the selected model's architecture:
	// MDX counts segment_size in STFT frames, Demucs in seconds. Zero means the
	// architecture default, so "not passed" is never confused with "set to 0".
	segSize := fs.Int("segment", 0, "separation segment_size (MDX: STFT frames, Demucs: seconds; 0 = default)")
	overlap := fs.Float64("overlap", 0, "separation window overlap fraction, e.g. 0.25 (0 = default)")
	shifts := fs.Int("shifts", 0, "Demucs random shifts (0 = default)")
	batch := fs.Int("batch", 0, "separation batch size (MDX/VR/MDXC; 0 = default)")
	fp16 := fs.Bool("fp16", false, "ask the torch stack for fp16 autocast (CUDA only, opt-in)")
	// Stems to keep, comma-separated (e.g. "vocals" or "vocals,instrumental").
	// Empty keeps everything the model produces. vocals is always required.
	keepStems := fs.String("stems", "", "stems to keep after separation, comma-separated (default: all; vocals always required)")

	// The knobs that matter on the first real use, reachable without the UI (F-10).
	lang := fs.String("language", "", "spoken language, e.g. zh or auto (default: engine default en)")
	translate := fs.Bool("translate", false, "translate the result into English")
	detectLang := fs.Bool("detect-language", false, "only report the detected language, no transcript")
	beam := fs.Int("beam-size", 0, "beam size 1-10 (0 keeps the engine default)")
	threads := fs.Int("threads", 0, "CPU threads (0 keeps the engine default)")
	vad := fs.Bool("vad", false, "voice activity detection (needs a VAD model)")
	prompt := fs.String("prompt", "", "seed text: terminology, names, style hints")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *noSeparate && !strings.EqualFold(*engineName, "synthetic") {
		// Separation is a mandatory stage (ADR-037). The flag survives only because the
		// smoke engine has no audio to separate; using it on the real path would
		// recreate exactly the silent skip this change removes.
		fmt.Fprintln(os.Stderr, "run: vocal separation is mandatory for the real engine; --no-separate is only valid with --engine synthetic")
		return 2
	}

	if *input == "" {
		fmt.Fprintln(os.Stderr, "run: --input is required")
		return 2
	}
	absInput, err := filepath.Abs(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: resolve input: %v\n", err)
		return 1
	}
	if info, statErr := os.Stat(absInput); statErr != nil {
		fmt.Fprintf(os.Stderr, "run: input not readable: %v\n", statErr)
		return 1
	} else if info.IsDir() {
		fmt.Fprintln(os.Stderr, "run: --input must be a file, not a directory")
		return 2
	}
	_ = *noPolish // polishing requires a provider; it is opt-in and never implicit

	// Start from the engine defaults and apply only what the user asked for. The
	// zero struct is not a safe baseline: it reads as flash-attention off among
	// other things, so defaults are the baseline and flags are deltas.
	params := engine.Defaults()
	if *lang != "" {
		params.InputLanguage = *lang
	}
	params.Translate = *translate
	params.DetectOnly = *detectLang
	if *beam > 0 {
		params.BeamSize = *beam
	}
	if *threads > 0 {
		params.Threads = *threads
	}
	params.VAD = *vad
	if *prompt != "" {
		params.InitialPrompt = *prompt
	}

	// Everything below is shared with the window and the browser shell: the
	// orchestration lives in internal/runctl so there is one run path, not three
	// (ADR-024). This function keeps only what is CLI-specific -- flags, human
	// readable printing and exit codes.
	emit := func(ev pipeline.Event) {
		switch ev.Kind {
		case "engine":
			fmt.Printf("engine: %s\n", ev.Note)
		case "plan.reason":
			fmt.Printf("  resume: %s\n", ev.Note)
		case "stage.start":
			fmt.Printf("→ %s\n", ev.Stage)
		case "stage.end":
			// Honour the OK flag: printing a tick for a failed stage is how a broken
			// run looks successful and sends the reader hunting in the wrong place.
			if ev.OK != nil && !*ev.OK {
				fmt.Printf("✗ %s (%s)\n", ev.Stage, ev.Note)
			} else {
				fmt.Printf("✓ %s\n", ev.Stage)
			}
		case "stage.warn":
			fmt.Printf("! %s (%s) %s\n", ev.Stage, ev.Code, ev.Note)
		case "stems":
			// Which tracks survived the retention choice. Printed because the files
			// are deleted, so the log is the only place the decision is visible.
			fmt.Printf("· %s %s\n", ev.Stage, ev.Note)
		case "stage.skip":
			fmt.Printf("· %s skipped (%s)\n", ev.Stage, ev.Note)
		}
	}

	// Ctrl+C has to reach the engine. With a background context the child kept
	// running after the console process died, holding VRAM until reboot.
	runCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignals()

	runctl.Version = version
	out, err := runctl.Run(runCtx, runctl.Options{
		Input:       absInput,
		Title:       *title,
		Workdir:     *workdir,
		ModelID:     *modelFlag,
		SeparatorID: *sepFlag,
		SepTuning: runctl.SeparateTuning{
			Segment: *segSize, Overlap: *overlap, Shifts: *shifts,
			BatchSize: *batch, FP16: *fp16,
		},
		ExportStems: splitCSV(*keepStems),
		Engine:      *engineName,
		DurationMS:  *durationMS,
		Params:      params,
		Separate:    !*noSeparate,
		Emit:        emit,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		return 1
	}

	fmt.Printf("\nbundle: %s\nsegments: %d\n", out.BundleDir, out.Segments)
	return 0
}

func loadSettings(wd string) (config.Settings, config.Layout, error) {
	return runctl.LoadSettings(wd)
}

// cmdProbe reports hardware and the backend that would be selected.
func cmdProbe(args []string) int {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	workdir := fs.String("workdir", "", "work directory (default: workdir next to the executable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	wd := *workdir
	if wd == "" {
		var err error
		if wd, err = config.DefaultWorkdir(); err != nil {
			fmt.Fprintf(os.Stderr, "probe: %v\n", err)
			return 1
		}
	}
	settings, _, err := loadSettings(wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "probe: %v\n", err)
		return 1
	}

	probe := resource.NewProbe()
	totalMB, totalErr := probe.TotalMB()
	freeMB, freeErr := probe.FreeMB()

	// One source of truth: the same resolver the GUI and a run use. Printing the
	// hardware preference here made probe say "cuda12" while the run used vulkan,
	// which is exactly the mismatch this change removes.
	bs := runctl.BackendReport(string(settings.Runtime.LockedBackend), wd)
	sched := resource.New(probe, settings.Runtime.ReserveMB)
	budget, budgetErr := sched.BudgetMB()

	installed := ""
	if bs.Installed {
		installed = " (installed " + bs.RuntimeVersion + ")"
	}
	fmt.Printf("backend      : %s%s\n", bs.Backend, installed)
	fmt.Printf("reason       : %s\n", bs.Reason)
	if name := resource.DeviceName(); name != "" {
		fmt.Printf("gpu          : %s\n", name)
	}
	if totalErr != nil {
		fmt.Printf("vram total   : unavailable (%v)\n", totalErr)
	} else {
		fmt.Printf("vram total   : %d MB\n", totalMB)
	}
	if freeErr != nil {
		fmt.Printf("vram free    : unavailable (%v)\n", freeErr)
	} else {
		fmt.Printf("vram free    : %d MB\n", freeMB)
	}
	if budgetErr != nil {
		fmt.Printf("vram budget  : unavailable (%v)\n", budgetErr)
	} else {
		fmt.Printf("vram budget  : %d MB (reserve %d MB)\n", budget, settings.Runtime.ReserveMB)
	}
	fmt.Printf("workdir      : %s\n", wd)
	return 0
}

// syntheticVideoID derives a stable id from path + size so re-running the same
// file updates one bundle instead of creating duplicates (ADR-006).
func syntheticVideoID(inputPath string) (string, error) {
	return runctl.VideoID(inputPath)
}

var errNotImplemented = errors.New("not implemented")

// splitCSV turns a comma-separated flag value into trimmed, non-empty entries.
// Validation (vocals required, names must belong to the model) happens in runctl,
// so the CLI does not duplicate the rule.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

var _ = store.HashParams
