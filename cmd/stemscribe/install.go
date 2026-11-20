package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/runctl"
)

// cmdRuntime handles "runtime install <id>".
func cmdRuntime(args []string) int {
	if len(args) < 2 || args[0] != "install" {
		fmt.Fprintln(os.Stderr, "usage: stemscribe runtime install <runtime-id>")
		return 2
	}
	return installOne(args[1], args[2:])
}

// cmdModel handles "model install <id>".
func cmdModel(args []string) int {
	if len(args) < 2 || args[0] != "install" {
		fmt.Fprintln(os.Stderr, "usage: stemscribe model install <model-id>")
		return 2
	}
	return installOne(args[1], args[2:])
}

// installOne keeps only what is CLI-specific: flags, printing and exit codes. The
// install itself lives in internal/runctl so the window and the browser can run the
// same code (the logic used to be here, where webapi could not reach it).
func installOne(id string, rest []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	workdir := fs.String("workdir", "", "work directory (default: workdir next to the executable)")
	// Parse only what follows the id: os.Args would also swallow the subcommand and
	// the id itself, which silently disabled --workdir. That was a real bug once.
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintf(os.Stderr, "install: %v\n", err)
		return 2
	}

	wd := *workdir
	if wd == "" {
		var err error
		if wd, err = config.DefaultWorkdir(); err != nil {
			fmt.Fprintf(os.Stderr, "install: %v\n", err)
			return 1
		}
	}

	// Progress is printed at the same points and in the same order as before the move,
	// so an operator's log stays comparable.
	onProgress := func(p runctl.Progress) {
		switch p.Stage {
		case "start":
			if p.Kind == "runtime" {
				fmt.Printf("installing runtime %s %s (%s)\n", p.ID, p.Version, p.Backend)
			} else {
				fmt.Printf("installing model %s\n", p.ID)
			}
			fmt.Printf("  from %s\n", p.SourceURL)
		case "verify":
			fmt.Println()
			shown := p.SHA256
			if len(shown) > 12 {
				shown = shown[:12]
			}
			fmt.Printf("  verified sha256 %s\n", shown)
		default:
			if p.TotalMB > 0 {
				fmt.Printf("  %3d%%  %d/%d MB\n", p.Percent, p.DoneMB, p.TotalMB)
			}
		}
	}

	done, err := runctl.Install(context.Background(), wd, id, onProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "install: %v\n", err)
		return 1
	}
	fmt.Printf("installed to %s\n", done.DestDir)
	return 0
}
