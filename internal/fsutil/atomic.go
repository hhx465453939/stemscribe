// Package fsutil provides the single atomic-write primitive used across the app.
//
// Why one implementation: the model downloader, the settings writer and the
// checkpoint writer all need the same guarantee — a crash must never leave a
// half-written file that a later run mistakes for a valid one. Duplicating that
// logic per caller is how "corrupted after power loss" bugs get born.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WriteFileAtomic writes data to path atomically within the same directory.
//
// The sequence is: create <path>.tmp -> write -> fsync -> close -> rename.
// Rename is atomic on NTFS and POSIX for same-volume moves, so readers either
// see the old file or the complete new one, never a partial write.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpName := tmp.Name()

	// Best-effort cleanup: on success the rename removes this name anyway.
	defer func() {
		if _, statErr := os.Stat(tmpName); statErr == nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp for %s: %w", path, err)
	}
	// Flush to disk before the rename, otherwise the rename can land while the
	// file contents are still in the page cache.
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp for %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp for %s: %w", path, err)
	}
	if err = os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp for %s: %w", path, err)
	}

	// Windows refuses rename onto an existing file in some cases; os.Rename
	// already handles the replace semantics on the supported platforms, but
	// remove first keeps behaviour uniform if the target is read-only.
	if _, statErr := os.Stat(path); statErr == nil {
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return fmt.Errorf("remove existing %s: %w", path, rmErr)
		}
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp to %s: %w", path, err)
	}
	return nil
}

// WriteStreamAtomic streams src into path with the same durability guarantee as
// WriteFileAtomic. It is used for large downloads, where buffering the whole
// payload in memory would be unacceptable.
func WriteStreamAtomic(path string, src io.Reader, perm os.FileMode) (int64, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("create dir for %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return 0, fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if _, statErr := os.Stat(tmpName); statErr == nil {
			_ = os.Remove(tmpName)
		}
	}()

	n, err := io.Copy(tmp, src)
	if err != nil {
		_ = tmp.Close()
		return n, fmt.Errorf("stream to temp for %s: %w", path, err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return n, fmt.Errorf("sync temp for %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return n, fmt.Errorf("close temp for %s: %w", path, err)
	}
	if err = os.Chmod(tmpName, perm); err != nil {
		return n, fmt.Errorf("chmod temp for %s: %w", path, err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return n, fmt.Errorf("remove existing %s: %w", path, rmErr)
		}
	}
	if err = os.Rename(tmpName, path); err != nil {
		return n, fmt.Errorf("rename temp to %s: %w", path, err)
	}
	return n, nil
}

// HasDotDotSegment reports whether any path segment is exactly "..".
//
// This must be called on the RAW path: filepath.Clean and filepath.Join both
// resolve ".." away, so checking the cleaned result can never detect the
// caller's intent to escape. Both separators are handled because a registry
// entry written on POSIX is read on Windows.
func HasDotDotSegment(p string) bool {
	if p == "" {
		return false
	}
	normalized := strings.ReplaceAll(p, "\\", "/")
	for _, seg := range strings.Split(normalized, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// SafeJoin joins base and a registry-supplied relative path, rejecting any
// attempt to escape base. Registry files are downloaded from the network, so a
// crafted `dest` field must never be able to write outside the work directory.
func SafeJoin(base, rel string) (string, error) {
	if rel == "" {
		return "", errors.New("empty relative path")
	}
	// Check the raw string first: cleaning happens later and would hide the intent.
	if HasDotDotSegment(rel) {
		return "", fmt.Errorf("path escapes base: %q", rel)
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
		return "", fmt.Errorf("absolute path not allowed: %q", rel)
	}
	if filepath.IsAbs(rel) || (len(rel) > 1 && rel[1] == ':') {
		return "", fmt.Errorf("absolute path not allowed: %q", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." {
		return "", fmt.Errorf("path resolves to base itself: %q", rel)
	}
	joined := filepath.Join(base, clean)
	// Defence in depth: verify the resolved path is still under base.
	relBack, err := filepath.Rel(base, joined)
	if err != nil {
		return "", fmt.Errorf("resolve %q under %q: %w", rel, base, err)
	}
	if relBack == ".." || strings.HasPrefix(relBack, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes base: %q", rel)
	}
	return joined, nil
}
