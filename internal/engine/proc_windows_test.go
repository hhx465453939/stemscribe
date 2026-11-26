//go:build windows

package engine

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A stopped run must not leave grandchildren behind: the separator's Python
// process spawns ffmpeg, and killing only the direct child keeps ffmpeg writing
// audio for a run the user already cancelled.
func TestWireCancelKillsGrandchildren(t *testing.T) {
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skip("python not on PATH")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The parent prints its child's PID, then sleeps. The child is the grandchild
	// relative to us.
	script := "import subprocess,sys,time; p=subprocess.Popen([sys.executable,'-c','import time;time.sleep(60)']); print(p.pid, flush=True); time.sleep(60)"
	cmd := exec.CommandContext(ctx, python, "-c", script)
	wireCancel(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("child pid %q: %v", line, err)
	}

	start := time.Now()
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process tree did not exit within 5s of cancel")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancel took %s", elapsed)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !pidAlive(t, childPID) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("grandchild pid %d is still alive after cancel", childPID)
}

func pidAlive(t *testing.T, pid int) bool {
	t.Helper()
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 2 {
			continue
		}
		if strings.Trim(fields[1], `"`) == strconv.Itoa(pid) {
			return true
		}
	}
	return false
}
