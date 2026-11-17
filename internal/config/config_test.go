package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAreUsable(t *testing.T) {
	s := Defaults(`D:\Stemscribe`)
	if s.Schema != CurrentSchema {
		t.Fatalf("schema = %d, want %d", s.Schema, CurrentSchema)
	}
	if !s.Stages.Separate {
		t.Error("separation should be on by default (PRD: 80% path defaults to on)")
	}
	if s.Stages.Polish {
		t.Error("polish must default to off: it sends text off-machine (N-05)")
	}
	if s.Runtime.ReserveMB < 1024 {
		t.Errorf("reserve = %d MB, want >= 1024 to survive compositor spikes", s.Runtime.ReserveMB)
	}
	if s.Stages.QualityTier != TierBalanced {
		t.Errorf("tier = %q, want %q", s.Stages.QualityTier, TierBalanced)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	l, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	s, err := l.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Workdir != l.Root {
		t.Errorf("workdir = %q, want %q", s.Workdir, l.Root)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	l, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	if err = l.Ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	want := Defaults(l.Root)
	want.Stages.Polish = true
	want.Stages.Whisper = "whisper-small-q5_0"
	want.Runtime.LockedBackend = BackendVulkan
	want.Runtime.MaxConcurrent = 2
	want.UI.Language = "en"

	if err = l.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := l.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.Stages.Whisper != want.Stages.Whisper {
		t.Errorf("whisper = %q, want %q", got.Stages.Whisper, want.Stages.Whisper)
	}
	if got.Stages.Polish != want.Stages.Polish {
		t.Errorf("polish = %v, want %v", got.Stages.Polish, want.Stages.Polish)
	}
	if got.Runtime.LockedBackend != want.Runtime.LockedBackend {
		t.Errorf("backend = %q, want %q", got.Runtime.LockedBackend, want.Runtime.LockedBackend)
	}
	if got.Runtime.MaxConcurrent != want.Runtime.MaxConcurrent {
		t.Errorf("concurrency = %d, want %d", got.Runtime.MaxConcurrent, want.Runtime.MaxConcurrent)
	}
}

func TestLoadRejectsFutureSchema(t *testing.T) {
	l, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	if err = l.Ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	// Schema 99 must fail loudly rather than silently dropping fields.
	if err = os.WriteFile(l.SettingsFile(), []byte(`{"schema":99,"workdir":"x"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err = l.Load(); err == nil {
		t.Fatal("expected schema mismatch error, got nil")
	}
}

func TestLoadFillsZeroedRuntimePrefs(t *testing.T) {
	l, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	if err = l.Ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	// A hand-edited file that dropped runtime knobs must not produce a 0 MB reserve.
	raw := `{"schema":1,"workdir":"` + filepath.ToSlash(l.Root) + `"}`
	if err = os.WriteFile(l.SettingsFile(), []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := l.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Runtime.ReserveMB <= 0 {
		t.Error("reserve must be repaired to a positive default")
	}
	if s.Runtime.MaxConcurrent <= 0 {
		t.Error("max_concurrent must be repaired to at least 1")
	}
}

func TestLayoutEnsureCreatesSkeleton(t *testing.T) {
	l, err := NewLayout(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	if err = l.Ensure(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	for _, dir := range []string{
		l.Config(), l.Runtimes(), l.Models(), l.Libraries(),
		l.Cache(), l.Logs(), l.Tmp(),
	} {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Errorf("%s not created: %v", dir, statErr)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
}

func TestNewLayoutRejectsEmpty(t *testing.T) {
	if _, err := NewLayout(""); err == nil {
		t.Fatal("expected error for empty workdir")
	}
}

func TestDefaultWorkdirIsAbsolute(t *testing.T) {
	dir, err := DefaultWorkdir()
	if err != nil {
		t.Fatalf("default workdir: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("workdir %q must be absolute", dir)
	}
}
