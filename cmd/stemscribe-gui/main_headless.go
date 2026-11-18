//go:build !(windows && gui)

// This file exists so `go build ./...` never fails on a machine without a C
// toolchain. The desktop shell is compiled only with "-tags gui"; everything else
// gets a binary that explains itself instead of a build error.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "The desktop shell is built with: pwsh scripts/build.ps1 -Desktop")
	fmt.Fprintln(os.Stderr, "It needs a C toolchain (cgo) and the WebView2 runtime.")
	fmt.Fprintln(os.Stderr, "The CLI is cmd/stemscribe and needs neither.")
	os.Exit(1)
}
