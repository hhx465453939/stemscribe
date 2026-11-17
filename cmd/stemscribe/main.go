// Command stemscribe is the single binary for the app.
//
// Launching it with no arguments (i.e. double-clicking the exe) opens the UI.
// Everything else is available as an explicit subcommand for scripts and CI,
// because every acceptance test in CHG-0001 §6 drives the CLI directly.
package main

import (
	"fmt"
	"os"
)

// version is stamped at build time via -ldflags "-X main.version=..."
var version = "0.0.1-dev"

func main() {
	// No arguments means "a human double-clicked this": open the app. Previously
	// this printed usage and exited 2, so the window vanished instantly and the
	// build looked broken.
	if len(os.Args) < 2 {
		os.Exit(cmdShell(nil))
	}

	switch os.Args[1] {
	case "shell":
		os.Exit(cmdShell(os.Args[2:]))
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "probe":
		os.Exit(cmdProbe(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Printf("stemscribe %s\n", version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `stemscribe `+version+`

Usage:
  stemscribe                        open the UI (same as: stemscribe shell)
  stemscribe shell   [--no-open]    serve the embedded UI on loopback and open it
  stemscribe run     --input <file> [--workdir <dir>] [--no-separate] [--duration-ms N]
  stemscribe probe   [--workdir <dir>]
  stemscribe version

Notes:
  The UI is served from 127.0.0.1 only and is embedded in this binary by
  scripts/build.ps1 (frontend/out -> cmd/stemscribe/assets).
  Inference is not part of this increment (CHG-0002); run uses a synthetic
  engine so the orchestration and artifacts can be verified.
`)
}
