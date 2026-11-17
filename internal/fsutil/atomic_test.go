package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := WriteFileAtomic(path, []byte("first"), 0o644); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := WriteFileAtomic(path, []byte("second"), 0o644); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("content = %q, want %q", got, "second")
	}
}

func TestWriteFileAtomicLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.bin")

	if err := WriteFileAtomic(path, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "target.bin" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("unexpected files left behind: %v", names)
	}
}

func TestWriteStreamAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stream.bin")
	payload := strings.Repeat("stemscribe", 1000)

	n, err := WriteStreamAtomic(path, strings.NewReader(payload), 0o644)
	if err != nil {
		t.Fatalf("stream write: %v", err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("bytes written = %d, want %d", n, len(payload))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("payload mismatch: got %d bytes", len(got))
	}
}

func TestSafeJoinRejectsEscapes(t *testing.T) {
	base := filepath.Join(t.TempDir(), "models")

	bad := []string{
		"",
		"..",
		"../etc/passwd",
		"..\\windows\\system32",
		"a/../../b",
		"/abs/path",
		"C:\\Windows\\system32",
		"models/../../escape",
	}
	for _, rel := range bad {
		if _, err := SafeJoin(base, rel); err == nil {
			t.Errorf("SafeJoin(%q) = nil error, want rejection", rel)
		}
	}
}

func TestSafeJoinAcceptsNormalPaths(t *testing.T) {
	base := filepath.Join(t.TempDir(), "models")

	good := []string{
		"ggml-medium-q5_0.bin",
		"whisper/medium/model.bin",
		"separators/uvr/Kim_Vocal_2.onnx",
	}
	for _, rel := range good {
		got, err := SafeJoin(base, rel)
		if err != nil {
			t.Errorf("SafeJoin(%q) unexpected error: %v", rel, err)
			continue
		}
		if !strings.HasPrefix(got, base) {
			t.Errorf("SafeJoin(%q) = %q, outside base %q", rel, got, base)
		}
	}
}
