// Package config owns the on-disk work directory layout and non-secret settings.
//
// Two rules drive the design:
//  1. Everything the app needs at runtime lives under one user-chosen root, so a
//     user can move or back up the whole thing by copying one folder (G-2).
//  2. Secrets never live here. They are encrypted via internal/vault (ADR-008).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/fsutil"
)

// CurrentSchema is bumped whenever Settings changes shape. Load() migrates or
// rejects; it never silently ignores unknown fields.
const CurrentSchema = 1

// QualityTier is the onboarding preset from PRD §4.1.
type QualityTier string

const (
	TierLight    QualityTier = "light"
	TierBalanced QualityTier = "balanced"
	TierMax      QualityTier = "max"
)

// Backend names the inference backend. "auto" lets the scheduler decide.
type Backend string

const (
	BackendAuto     Backend = "auto"
	BackendCUDA12   Backend = "cuda12"
	BackendVulkan   Backend = "vulkan"
	BackendDirectML Backend = "directml"
	BackendCPU      Backend = "cpu"
)

// Settings is the non-secret configuration persisted to <workdir>/config/settings.json.
type Settings struct {
	Schema  int    `json:"schema"`
	Workdir string `json:"workdir"`
	// OutputDir is where finished bundles land. Empty keeps the historical
	// workdir/libraries/<videoID> layout; set, it becomes <OutputDir>/<videoID>/.
	// The per-video self-contained directory is unchanged either way (ADR-029).
	OutputDir string `json:"output_dir"`

	UI      UIPrefs      `json:"ui"`
	Stages  StagePrefs   `json:"stages"`
	Runtime RuntimePrefs `json:"runtime"`
}

// UIPrefs holds presentation-only preferences.
type UIPrefs struct {
	Language     string `json:"language"`      // "zh-CN" | "en"
	ReduceMotion bool   `json:"reduce_motion"` // honour prefers-reduced-motion by default
}

// StagePrefs holds pipeline defaults applied to new tasks.
type StagePrefs struct {
	Separate   bool   `json:"separate"`
	Whisper    string `json:"whisper_model"`
	Polish     bool   `json:"polish"`
	RetainRaw  bool   `json:"retain_raw_video"`
	QualityTier QualityTier `json:"quality_tier"`
}

// RuntimePrefs holds engine/resource knobs.
type RuntimePrefs struct {
	// LockedBackend pins a backend and disables automatic downgrade (ADR-004).
	LockedBackend Backend `json:"locked_backend"`
	// ReserveMB is VRAM kept away from inference for the compositor and driver.
	ReserveMB int `json:"reserve_mb"`
	// MaxConcurrent caps simultaneous tasks regardless of VRAM.
	MaxConcurrent int `json:"max_concurrent"`
	// MirrorID selects an entry from the registry mirrors list; empty = auto.
	MirrorID string `json:"mirror_id"`
}

// Defaults returns the "just works" configuration for the given workdir.
func Defaults(workdir string) Settings {
	return Settings{
		Schema:  CurrentSchema,
		Workdir: workdir,
		UI: UIPrefs{
			Language:     "zh-CN",
			ReduceMotion: false,
		},
		Stages: StagePrefs{
			Separate:    true,
			Whisper:     "whisper-medium",
			Polish:      false,
			RetainRaw:   true,
			QualityTier: TierBalanced,
		},
		Runtime: RuntimePrefs{
			LockedBackend: BackendAuto,
			ReserveMB:     1536,
			MaxConcurrent: 1,
			MirrorID:      "",
		},
	}
}

// DefaultWorkdir returns "workdir" next to the executable.
//
// Why not the user profile: each model is 0.5-3 GB. Scattering them under
// %USERPROFILE% makes the install impossible to move, back up or wipe as a unit,
// and leaves gigabytes behind after uninstall. Next to the exe the whole thing is
// one portable folder, which is also what the registry promised (PRD F-09).
//
// Development note: `go run` puts the binary under the OS temp dir, so an
// exe-relative default would land in temp. In that case fall back to ./workdir in
// the current directory, which is the repository during development.
func DefaultWorkdir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	dir := filepath.Dir(exe)

	if isUnderTempDir(dir) {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return "", fmt.Errorf("locate working directory: %w", cwdErr)
		}
		return filepath.Join(cwd, "workdir"), nil
	}
	return filepath.Join(dir, "workdir"), nil
}

// isUnderTempDir reports whether dir sits inside the OS temp directory.
func isUnderTempDir(dir string) bool {
	tmp := filepath.Clean(os.TempDir())
	rel, err := filepath.Rel(tmp, filepath.Clean(dir))
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

// Layout maps the fixed sub-directory contract from docs/MODEL-REGISTRY.md §4.
type Layout struct {
	Root string
}

// NewLayout validates and normalises a work directory path.
func NewLayout(root string) (Layout, error) {
	if root == "" {
		return Layout{}, errors.New("workdir must not be empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Layout{}, fmt.Errorf("abs workdir %q: %w", root, err)
	}
	return Layout{Root: abs}, nil
}

func (l Layout) Config() string    { return filepath.Join(l.Root, "config") }
func (l Layout) Runtimes() string  { return filepath.Join(l.Root, "runtimes") }
func (l Layout) Models() string    { return filepath.Join(l.Root, "models") }
func (l Layout) Libraries() string { return filepath.Join(l.Root, "libraries") }
func (l Layout) Cache() string     { return filepath.Join(l.Root, "cache") }
func (l Layout) Logs() string      { return filepath.Join(l.Root, "logs") }
func (l Layout) Tmp() string       { return filepath.Join(l.Root, "tmp") }

func (l Layout) SettingsFile() string { return filepath.Join(l.Config(), "settings.json") }
func (l Layout) SecretsFile() string  { return filepath.Join(l.Config(), "secrets.bin") }
func (l Layout) ProvidersFile() string {
	return filepath.Join(l.Config(), "providers.json")
}
func (l Layout) CalibrationFile() string {
	return filepath.Join(l.Config(), "calibration.json")
}
func (l Layout) InstallLock() string {
	return filepath.Join(l.Config(), "models.lock.json")
}

// Ensure creates the directory skeleton. Safe to call on every startup.
func (l Layout) Ensure() error {
	dirs := []string{
		l.Config(), l.Runtimes(), l.Models(), l.Libraries(),
		l.Cache(), l.Logs(), l.Tmp(),
		filepath.Join(l.Cache(), "peaks"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	return nil
}

// Load reads settings.json, or returns defaults when the file does not exist.
// A schema mismatch is an error rather than a silent coercion: losing a user's
// configuration quietly is worse than failing loudly once.
func (l Layout) Load() (Settings, error) {
	path := l.SettingsFile()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(l.Root), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read %s: %w", path, err)
	}

	var s Settings
	if err = json.Unmarshal(raw, &s); err != nil {
		return Settings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if s.Schema == 0 {
		// Hand-edited or truncated file: fall back to defaults but keep workdir.
		s = Defaults(l.Root)
	}
	if s.Schema != CurrentSchema {
		return Settings{}, fmt.Errorf("settings schema %d unsupported (want %d)", s.Schema, CurrentSchema)
	}
	if s.Workdir == "" {
		s.Workdir = l.Root
	}
	if s.Runtime.ReserveMB <= 0 {
		s.Runtime.ReserveMB = Defaults(l.Root).Runtime.ReserveMB
	}
	if s.Runtime.MaxConcurrent <= 0 {
		s.Runtime.MaxConcurrent = 1
	}
	if s.Stages.QualityTier == "" {
		s.Stages.QualityTier = TierBalanced
	}
	if s.UI.Language == "" {
		s.UI.Language = "zh-CN"
	}
	return s, nil
}

// Save writes settings atomically.
func (l Layout) Save(s Settings) error {
	s.Schema = CurrentSchema
	if s.Workdir == "" {
		s.Workdir = l.Root
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	raw = append(raw, '\n')
	if err = fsutil.WriteFileAtomic(l.SettingsFile(), raw, 0o644); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}
