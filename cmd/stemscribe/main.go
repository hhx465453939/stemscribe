// Command stemscribe is the single binary for the app: it hosts the desktop UI
// (Wails, later increment) and the headless CLI used by smoke tests and CI.
//
// The CLI exists because the pipeline must be verifiable without a GUI: every
// acceptance test in CHG-0001 §6 drives it directly.
package main

import (
	"fmt"
	"os"
)

// version is stamped at build time via -ldflags "-X main.version=..."
var version = "0.0.1-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
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
  stemscribe run     --input <file> [--workdir <dir>] [--no-separate] [--no-polish] [--title <t>]
  stemscribe probe   [--workdir <dir>]
  stemscribe version

Notes:
  run      executes the pipeline for a local media file and writes a bundle
  probe    reports the detected hardware and the backend that would be used
  Engine inference is not part of this increment (CHG-0002); run uses a
  synthetic engine so the orchestration and artifacts can be verified.
`)
}
