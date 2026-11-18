package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/pipeline"
	"github.com/hhx465453939/stemscribe/internal/resource"
	"github.com/hhx465453939/stemscribe/internal/store"
)

// cmdRun executes the pipeline for one local file.
func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	input := fs.String("input", "", "path to a local media file")
	workdir := fs.String("workdir", "", "work directory (default: user profile/Stemscribe)")
	title := fs.String("title", "", "display title for the bundle")
	noSeparate := fs.Bool("no-separate", false, "skip vocal separation")
	noPolish := fs.Bool("no-polish", false, "skip LLM polishing (default: already skipped)")
	durationMS := fs.Int("duration-ms", 30000, "synthetic duration used by the smoke engine")
	engineName := fs.String("engine", "auto", "engine: auto (real whisper.cpp) | synthetic (test double)")
	if err := fs.Parse(args); err != nil {
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

	wd := *workdir
	if wd == "" {
		if wd, err = config.DefaultWorkdir(); err != nil {
			fmt.Fprintf(os.Stderr, "run: resolve default workdir: %v\n", err)
			return 1
		}
	}
	settings, layout, err := loadSettings(wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		return 1
	}

	// "auto" resolves the real engine from the workdir; "synthetic" is the
	// documented test double that does not read audio.
	if *engineName != "auto" && *engineName != "synthetic" {
		fmt.Fprintf(os.Stderr, "run: unknown engine %q (use auto or synthetic)\n", *engineName)
		return 2
	}

	videoID, err := syntheticVideoID(absInput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: derive video id: %v\n", err)
		return 1
	}
	bundleDir := filepath.Join(layout.Libraries(), videoID)
	bundle, err := store.New(bundleDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: open bundle: %v\n", err)
		return 1
	}
	if err = bundle.Ensure(); err != nil {
		fmt.Fprintf(os.Stderr, "run: prepare bundle: %v\n", err)
		return 1
	}

	runID := time.Now().UTC().Format("20060102T150405Z")
	runLog, err := os.Create(filepath.Join(bundle.Dir(), "runs", runID+".jsonl"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: create run log: %v\n", err)
		return 1
	}
	defer func() { _ = runLog.Close() }()

	emit := func(ev pipeline.Event) {
		if ev.TS == "" {
			ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
		}
		raw, marshalErr := json.Marshal(ev)
		if marshalErr != nil {
			return
		}
		_, _ = runLog.Write(append(raw, '\n'))
		// Human-readable progress on stdout; machine consumers read the JSONL file.
		switch ev.Kind {
		case "stage.start":
			fmt.Printf("→ %s\n", ev.Stage)
		case "stage.end":
			fmt.Printf("✓ %s\n", ev.Stage)
		case "stage.warn":
			fmt.Printf("! %s (%s) %s\n", ev.Stage, ev.Code, ev.Note)
		case "stage.skip":
			fmt.Printf("· %s skipped (%s)\n", ev.Stage, ev.Note)
		}
	}

	separate := settings.Stages.Separate && !*noSeparate
	disabled := map[store.Stage]bool{
		store.StageSeparate: !separate,
		store.StagePolish:   true,
	}

	cp, err := bundle.ReadCheckpoint(videoID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: read checkpoint: %v\n", err)
		return 1
	}
	plan := cp.Plan(bundle.Observe(), nil, disabled)
	for _, reason := range plan.Reasons {
		fmt.Printf("  resume: %s\n", reason)
	}

	eng, engDesc, engErr := buildEngine(*engineName, layout.Root, settings.Stages.Whisper,
		string(settings.Runtime.LockedBackend), *durationMS)
	if engErr != nil {
		fmt.Fprintln(os.Stderr, engErr)
		return 1
	}
	fmt.Printf("engine: %s\n", engDesc)

	// Separation moved to CHG-0003 (ADR-014). Saying so once and continuing keeps an
	// unavailable optional stage from failing an otherwise fine transcription.
	if separate {
		if _, isSynth := eng.(*syntheticEngine); !isSynth {
			fmt.Println("note: vocal separation is not available yet (CHG-0003); transcribing the original audio")
			separate = false
			disabled[store.StageSeparate] = true
		}
	}

	runner := &pipeline.Runner{Engine: eng, Emit: emit}
	result, err := runner.Run(context.Background(), pipeline.Request{
		Bundle:       bundle,
		Plan:         plan,
		VideoID:      videoID,
		Separate:     separate,
		Transcribe:   pipeline.TranscribeOptions{Model: settings.Stages.Whisper, VAD: true},
		SeparateOpts: pipeline.SeparateOptions{SegmentSecs: 15, OverlapSecs: 2, HalfPrecision: true},
	})
	if err != nil {
		meta := buildMeta(videoID, *title, absInput, "failed")
		meta.Stages[string(result.FailedAt)] = store.StageRun{State: "failed", Message: err.Error()}
		_ = bundle.WriteMeta(meta)
		fmt.Fprintf(os.Stderr, "run: failed at %s: %v\n", result.FailedAt, err)
		return 1
	}

	segs, err := pipeline.ReadTranscript(filepath.Join(bundle.Dir(), "text", "transcript.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: read transcript: %v\n", err)
		return 1
	}
	if err = writeArtifacts(bundle, *title, segs); err != nil {
		fmt.Fprintf(os.Stderr, "run: export: %v\n", err)
		return 1
	}

	meta := buildMeta(videoID, *title, absInput, "done")
	for _, s := range result.Completed {
		meta.Stages[string(s)] = store.StageRun{State: "ok"}
	}
	if err = bundle.WriteMeta(meta); err != nil {
		fmt.Fprintf(os.Stderr, "run: write meta: %v\n", err)
		return 1
	}

	fmt.Printf("\nbundle: %s\nsegments: %d\n", bundle.Dir(), len(segs))
	return 0
}

func loadSettings(wd string) (config.Settings, config.Layout, error) {
	layout, err := config.NewLayout(wd)
	if err != nil {
		return config.Settings{}, config.Layout{}, err
	}
	if err = layout.Ensure(); err != nil {
		return config.Settings{}, config.Layout{}, err
	}
	settings, err := layout.Load()
	if err != nil {
		return config.Settings{}, config.Layout{}, err
	}
	return settings, layout, nil
}

func buildMeta(videoID, title, inputPath, status string) store.Meta {
	if title == "" {
		title = filepath.Base(inputPath)
	}
	return store.Meta{
		VideoID: videoID,
		Title:   title,
		Status:  status,
		Source:  store.Source{Kind: "file", Origin: "local"},
		Stages:  map[string]store.StageRun{},
		Provenance: store.Provenance{
			Engines: map[string]store.EngineInfo{
				"transcribe": {Name: "synthetic", Version: version},
			},
			ParamsSnapshotHash: store.HashParams("synthetic", version),
		},
	}
}

func writeArtifacts(b *store.Bundle, title string, segs []pipeline.Segment) error {
	if title == "" {
		title = b.Dir()
	}
	files := map[string]string{
		"text/transcript.srt": pipeline.ToSRT(segs),
		"text/transcript.vtt": pipeline.ToVTT(segs),
		"text/transcript.md":  pipeline.ToMarkdown(title, segs),
	}
	for rel, content := range files {
		full, err := b.Path(rel)
		if err != nil {
			return err
		}
		if err = os.WriteFile(full, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}

// cmdProbe reports hardware and the backend that would be selected.
func cmdProbe(args []string) int {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	workdir := fs.String("workdir", "", "work directory (default: user profile/Stemscribe)")
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

	hw := resource.Hardware{}
	if totalErr == nil && totalMB > 0 {
		hw.HasNVIDIA = resource.LooksNVIDIA()
		hw.DriverMajor = resource.DriverMajor()
		hw.CUDA12Available = resource.CUDA12Present()
	}
	hw.VulkanAvailable = resource.VulkanPresent()

	sel := resource.SelectBackend(hw, resource.Backend(settings.Runtime.LockedBackend))
	sched := resource.New(probe, settings.Runtime.ReserveMB)
	budget, budgetErr := sched.BudgetMB()

	fmt.Printf("backend      : %s\n", sel.Backend)
	fmt.Printf("reason       : %s\n", sel.Reason)
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
	info, err := os.Stat(inputPath)
	if err != nil {
		return "", err
	}
	return "local-" + store.HashParams(inputPath, fmt.Sprint(info.Size()))[4:], nil
}

var errNotImplemented = errors.New("not implemented")
