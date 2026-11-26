package runctl

import (
	"fmt"
	"os"
	"strings"
)

// CheckOutputDir proves a directory can be created and written to before a run
// starts. Without this the failure surfaced after separation (minutes in), with
// the bundle half built and the user looking at a generic write error.
//
// An empty value is valid: it means "use <workdir>/libraries", which the layout
// already owns.
func CheckOutputDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot use that folder: %w", err)
	}
	// MkdirAll succeeds on an existing directory even when this user cannot write
	// to it, so the only honest check is to actually create a file.
	f, err := os.CreateTemp(dir, ".stemscribe-write-test-*")
	if err != nil {
		return fmt.Errorf("that folder is not writable: %w", err)
	}
	name := f.Name()
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(name)
		return fmt.Errorf("that folder is not writable: %w", closeErr)
	}
	if rmErr := os.Remove(name); rmErr != nil {
		return fmt.Errorf("that folder is not writable: %w", rmErr)
	}
	return nil
}
