//go:build !windows

package engine

import (
	"os/exec"
	"time"
)

// wireCancel keeps the default kill on non-Windows. The shipped client is Windows;
// this file only keeps `go test` working on other hosts.
func wireCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 3 * time.Second
}
