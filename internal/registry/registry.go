// Package registry loads and validates the model/runtime catalog.
//
// The catalog ships with the app (ADR-003) so that a dead network never blocks
// startup. Validation is strict on purpose: this file is the only thing standing
// between a hand-edited catalog and writing files into arbitrary paths.
package registry

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed data/registry.json
var embeddedCatalog []byte

// Builtin returns the catalog compiled into the binary.
func Builtin() (Registry, error) {
	return Parse(embeddedCatalog)
}

// Engine names a supported inference engine.
type Engine string

const (
	EngineWhisper   Engine = "whisper"
	EngineSeparator Engine = "separator"
	EngineFFmpeg    Engine = "ffmpeg"
)

// Backend names a compiled acceleration backend.
type Backend string

const (
	BackendCUDA12   Backend = "cuda12"
	BackendCUDA11   Backend = "cuda11"
	BackendVulkan   Backend = "vulkan"
	BackendDirectML Backend = "directml"
	BackendCPU      Backend = "cpu"
	BackendNone     Backend = "none"
)

// License records the upstream licence of a downloadable artifact.
type License struct {
	SPDX string `json:"spdx"`
	URL  string `json:"url,omitempty"`
	Note string `json:"note,omitempty"`
}

// FileRef is one downloadable artifact.
type FileRef struct {
	URL    string  `json:"url"`
	Size   int64   `json:"size"`
	SHA256 *string `json:"sha256"`
	TBD    bool    `json:"tbd,omitempty"`
	Dest   string  `json:"dest"`
}

// Mirror is an alternate download root, ordered by priority (lower first).
type Mirror struct {
	ID       string `json:"id"`
	BaseURL  string `json:"base_url"`
	Priority int    `json:"priority"`
	Note     string `json:"note,omitempty"`
}

// Requires records driver/runtime prerequisites for a backend build.
type Requires struct {
	CUDA      string `json:"cuda,omitempty"`
	Driver    string `json:"driver,omitempty"`
	VCRuntime string `json:"vcruntime,omitempty"`
}

// Runtime is a downloadable engine build.
type Runtime struct {
	ID             string   `json:"id"`
	Engine         Engine   `json:"engine"`
	Version        string   `json:"version"`
	Backend        Backend  `json:"backend"`
	Platform       string   `json:"platform"`
	Archive        FileRef  `json:"archive"`
	Entrypoint     string   `json:"entrypoint"`
	Requires       Requires `json:"requires,omitempty"`
	VRAMOverheadMB int      `json:"vram_overhead_mb"`
	License        License  `json:"license"`
}

// Model is a downloadable weight file (no executable code).
type Model struct {
	ID                 string   `json:"id"`
	Family             string   `json:"family"`
	Engine             Engine   `json:"engine"`
	DisplayName        string   `json:"display_name"`
	Langs              []string `json:"langs,omitempty"`
	// Stems is what a separator model can actually output. MDX-Net and VR are
	// two-stem (vocals/instrumental); Demucs is four. A global on/off switch cannot
	// express that, which is why the capability lives on the model (ADR-019).
	Stems              []string `json:"stems,omitempty"`
	Files              []FileRef `json:"files"`
	VRAMEstimateMB     int      `json:"vram_estimate_mb"`
	RAMEstimateMB      int      `json:"ram_estimate_mb,omitempty"`
	DiskAfterInstallMB int      `json:"disk_after_install_mb"`
	QualityTier        string   `json:"quality_tier"`
	SegmentDefaultS    int      `json:"segment_default_s,omitempty"`
	Quantization       string   `json:"quantization,omitempty"`
	License            License  `json:"license"`
	Source             string   `json:"source"`
	Notes              string   `json:"notes,omitempty"`
}

// Registry is the parsed catalog.
type Registry struct {
	Schema      int       `json:"schema"`
	GeneratedAt string    `json:"generated_at"`
	Mirrors     []Mirror  `json:"mirrors"`
	Runtimes    []Runtime `json:"runtimes"`
	Models      []Model   `json:"models"`
}

var (
	sha256Re   = regexp.MustCompile(`^[a-f0-9]{64}$`)
	// Versions are either semver or an upstream build id: ffmpeg publishes
	// N-129046 rather than a semver, and refusing it would mean either lying about
	// the version or not shipping ffmpeg at all. Kept bounded so junk still fails.
	versionRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,31}$`)
	httpURLRe  = regexp.MustCompile(`^https?://`)
	relPathRe  = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	platforms  = map[string]bool{"windows-x64": true, "windows-arm64": true, "darwin-arm64": true, "linux-x64": true}
	backends   = map[Backend]bool{BackendCUDA12: true, BackendCUDA11: true, BackendVulkan: true, BackendDirectML: true, BackendCPU: true, BackendNone: true}
	engines    = map[Engine]bool{EngineWhisper: true, EngineSeparator: true, EngineFFmpeg: true}
	tiers      = map[string]bool{"light": true, "balanced": true, "max": true}
)

// Parse decodes and validates a catalog.
func Parse(raw []byte) (Registry, error) {
	var r Registry
	if err := json.Unmarshal(raw, &r); err != nil {
		return Registry{}, fmt.Errorf("parse registry: %w", err)
	}
	if err := r.Validate(); err != nil {
		return Registry{}, err
	}
	return r, nil
}

// Validate enforces the contract from docs/MODEL-REGISTRY.md §2.1/§3.
func (r Registry) Validate() error {
	if r.Schema != 1 {
		return fmt.Errorf("registry schema %d unsupported (want 1)", r.Schema)
	}
	if len(r.Mirrors) == 0 {
		return fmt.Errorf("registry has no mirrors")
	}
	for i, m := range r.Mirrors {
		if m.ID == "" {
			return fmt.Errorf("mirrors[%d]: empty id", i)
		}
		if !httpURLRe.MatchString(m.BaseURL) {
			return fmt.Errorf("mirrors[%d] (%s): base_url must be http(s), got %q", i, m.ID, m.BaseURL)
		}
	}
	if len(r.Runtimes) == 0 {
		return fmt.Errorf("registry has no runtimes")
	}
	for i, rt := range r.Runtimes {
		if err := validateRuntime(rt); err != nil {
			return fmt.Errorf("runtimes[%d] (%s): %w", i, rt.ID, err)
		}
	}
	if len(r.Models) == 0 {
		return fmt.Errorf("registry has no models")
	}
	seen := map[string]bool{}
	for i, m := range r.Models {
		if err := validateModel(m); err != nil {
			return fmt.Errorf("models[%d] (%s): %w", i, m.ID, err)
		}
		if seen[m.ID] {
			return fmt.Errorf("models[%d]: duplicate id %q", i, m.ID)
		}
		seen[m.ID] = true
	}
	return nil
}

func validateFileRef(prefix string, f FileRef) error {
	if f.URL == "" {
		return fmt.Errorf("%s: empty url", prefix)
	}
	if f.Size <= 0 {
		return fmt.Errorf("%s: size must be positive, got %d", prefix, f.Size)
	}
	if f.SHA256 == nil {
		// Rule: unmeasured checksums must be explicitly marked TBD. Never invent one.
		if !f.TBD {
			return fmt.Errorf("%s: sha256 is null but tbd is not set", prefix)
		}
	} else {
		if f.TBD {
			return fmt.Errorf("%s: sha256 set but tbd also set", prefix)
		}
		if !sha256Re.MatchString(*f.SHA256) {
			return fmt.Errorf("%s: sha256 must be 64 lowercase hex chars", prefix)
		}
	}
	if err := validateRelPath(prefix+".dest", f.Dest); err != nil {
		return err
	}
	return nil
}

func validateRelPath(prefix, p string) error {
	if p == "" {
		return fmt.Errorf("%s: empty path", prefix)
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return fmt.Errorf("%s: absolute path not allowed: %q", prefix, p)
	}
	if len(p) > 1 && p[1] == ':' {
		return fmt.Errorf("%s: drive path not allowed: %q", prefix, p)
	}
	if !relPathRe.MatchString(p) {
		return fmt.Errorf("%s: illegal characters in %q", prefix, p)
	}
	for _, seg := range strings.Split(strings.ReplaceAll(p, "\\", "/"), "/") {
		if seg == ".." {
			return fmt.Errorf("%s: traversal not allowed: %q", prefix, p)
		}
	}
	return nil
}

func validateRuntime(rt Runtime) error {
	if rt.ID == "" {
		return fmt.Errorf("empty id")
	}
	if !engines[rt.Engine] {
		return fmt.Errorf("unknown engine %q", rt.Engine)
	}
	if !versionRe.MatchString(rt.Version) {
		return fmt.Errorf("invalid version %q", rt.Version)
	}
	if !backends[rt.Backend] {
		return fmt.Errorf("unknown backend %q", rt.Backend)
	}
	if !platforms[rt.Platform] {
		return fmt.Errorf("unknown platform %q", rt.Platform)
	}
	if err := validateFileRef("archive", rt.Archive); err != nil {
		return err
	}
	if err := validateRelPath("entrypoint", rt.Entrypoint); err != nil {
		return err
	}
	if rt.VRAMOverheadMB < 0 {
		return fmt.Errorf("vram_overhead_mb must be >= 0")
	}
	if rt.License.SPDX == "" {
		return fmt.Errorf("license.spdx is required (redistribution compliance)")
	}
	return nil
}

// contains reports whether want is in the slice.
func contains(hay []string, want string) bool {
	for _, s := range hay {
		if s == want {
			return true
		}
	}
	return false
}

func validateModel(m Model) error {
	if m.ID == "" {
		return fmt.Errorf("empty id")
	}
	if !engines[m.Engine] {
		return fmt.Errorf("unknown engine %q", m.Engine)
	}
	if m.Family != string(EngineWhisper) && m.Family != string(EngineSeparator) {
		return fmt.Errorf("unknown family %q", m.Family)
	}
	if m.Family == string(EngineSeparator) {
		// Without declared stems the UI cannot know whether to offer 2 or 4 outputs,
		// and a wrong guess silently drops tracks the model did produce.
		if len(m.Stems) < 2 {
			return fmt.Errorf("separator models must declare at least two stems, got %d", len(m.Stems))
		}
		valid := map[string]bool{"vocals": true, "instrumental": true, "drums": true, "bass": true, "other": true}
		for _, s := range m.Stems {
			if !valid[s] {
				return fmt.Errorf("unknown stem %q", s)
			}
		}
		if !contains(m.Stems, "vocals") {
			return fmt.Errorf("separator models must include the vocals stem")
		}
	}
	if m.DisplayName == "" {
		return fmt.Errorf("display_name is required")
	}
	if len(m.Files) == 0 {
		return fmt.Errorf("no files")
	}
	for i, f := range m.Files {
		if err := validateFileRef(fmt.Sprintf("files[%d]", i), f); err != nil {
			return err
		}
	}
	// VRAM is only meaningful for engines that actually use the GPU. The separator
	// runs on the CPU onnxruntime provider today, where 0 is the truthful value and
	// a positive one would be an invented budget.
	if m.VRAMEstimateMB < 0 {
		return fmt.Errorf("vram_estimate_mb must be >= 0, got %d", m.VRAMEstimateMB)
	}
	if m.VRAMEstimateMB == 0 && m.Family == string(EngineWhisper) {
		return fmt.Errorf("vram_estimate_mb must be positive for whisper models")
	}
	if m.DiskAfterInstallMB <= 0 {
		return fmt.Errorf("disk_after_install_mb must be positive")
	}
	if !tiers[m.QualityTier] {
		return fmt.Errorf("unknown quality_tier %q", m.QualityTier)
	}
	if m.License.SPDX == "" {
		return fmt.Errorf("license.spdx is required (no silent weight redistribution)")
	}
	if !httpURLRe.MatchString(m.Source) {
		return fmt.Errorf("source must be an http(s) URL")
	}
	return nil
}

// FindModel returns the model with the given id.
func (r Registry) FindModel(id string) (Model, bool) {
	for _, m := range r.Models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// FindRuntime returns the runtime build for engine+backend, if present.
func (r Registry) FindRuntime(engine Engine, backend Backend, platform string) (Runtime, bool) {
	for _, rt := range r.Runtimes {
		if rt.Engine == engine && rt.Backend == backend && rt.Platform == platform {
			return rt, true
		}
	}
	return Runtime{}, false
}

// ModelsFor returns every model belonging to the given engine.
func (r Registry) ModelsFor(engine Engine) []Model {
	var out []Model
	for _, m := range r.Models {
		if m.Engine == engine {
			out = append(out, m)
		}
	}
	return out
}

// MirrorByID returns a mirror by id.
func (r Registry) MirrorByID(id string) (Mirror, bool) {
	for _, m := range r.Mirrors {
		if m.ID == id {
			return m, true
		}
	}
	return Mirror{}, false
}

// ResolveURL turns a catalog-relative file URL into an absolute download URL for
// the given mirror. Absolute URLs in the catalog are returned unchanged.
func (r Registry) ResolveURL(m Mirror, fileURL string) string {
	if httpURLRe.MatchString(fileURL) {
		return fileURL
	}
	return strings.TrimRight(m.BaseURL, "/") + "/" + strings.TrimLeft(fileURL, "/")
}
