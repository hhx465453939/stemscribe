//go:build windows

package engine

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

// wireCancel replaces exec.CommandContext's default "kill the direct child" with
// a process-tree kill. whisper-cli is a leaf, but the separator's Python process
// spawns ffmpeg: killing only python leaves ffmpeg writing a wav for a run the
// user already stopped.
func wireCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return killTree(cmd.Process) }
	// If taskkill cannot make the tree exit, Wait must still return rather than
	// hang the worker forever.
	cmd.WaitDelay = 3 * time.Second
}

func killTree(p *os.Process) error {
	if p == nil {
		return nil
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.Pid))
	if err := kill.Run(); err != nil {
		// taskkill can fail when the process already exited; fall back so a race
		// does not turn a completed run into an error.
		return p.Kill()
	}
	return nil
}
