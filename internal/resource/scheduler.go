// Package resource decides what may be loaded into VRAM and what has to be
// evicted first (ADR-004).
//
// The scheduler is deliberately free of any OS or GPU call: all hardware facts
// arrive through the Probe interface, which makes every branch unit-testable
// without a GPU (AC-12).
package resource

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Probe reports VRAM in mebibytes.
type Probe interface {
	TotalMB() (int, error)
	FreeMB() (int, error)
}

// ErrProbeUnavailable means hardware facts could not be read; callers must fall
// back to the CPU path rather than guessing.
var ErrProbeUnavailable = errors.New("vram probe unavailable")

// Entry is a model currently occupying VRAM.
type Entry struct {
	Key      string
	VRAMMB   int
	LastUsed time.Time
	// Pinned entries belong to a running task and must never be evicted.
	Pinned bool
}

// Decision is the outcome of an admission request.
type Decision struct {
	Allowed  bool
	Evict    []string
	Reason   string
	FreeMB   int
	BudgetMB int
	NeedMB   int
}

// Scheduler applies the VRAM budget and LRU eviction policy.
type Scheduler struct {
	probe     Probe
	reserveMB int
	now       func() time.Time
	resident  map[string]*Entry
}

// New builds a scheduler. A nil probe models a CPU-only machine.
func New(probe Probe, reserveMB int) *Scheduler {
	if reserveMB < 0 {
		reserveMB = 0
	}
	return &Scheduler{
		probe:     probe,
		reserveMB: reserveMB,
		now:       time.Now,
		resident:  map[string]*Entry{},
	}
}

// ReserveMB reports the configured reserve.
func (s *Scheduler) ReserveMB() int { return s.reserveMB }

// BudgetMB is free VRAM minus the reserve, clamped at zero.
//
// This is the number the UI shows and the number admission is judged against, so
// it must never be a guess: a missing probe is an error, not "assume 0".
func (s *Scheduler) BudgetMB() (int, error) {
	if s == nil || s.probe == nil {
		return 0, fmt.Errorf("%w: no probe (cpu-only build)", ErrProbeUnavailable)
	}
	free, err := s.probe.FreeMB()
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrProbeUnavailable, err)
	}
	if _, err = s.probe.TotalMB(); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrProbeUnavailable, err)
	}
	budget := free - s.reserveMB
	if budget < 0 {
		budget = 0
	}
	return budget, nil
}

// Admit decides whether a model needing needMB may be loaded, and which resident
// models must be evicted first.
func (s *Scheduler) Admit(needMB int) (Decision, error) {
	d := Decision{NeedMB: needMB}
	if needMB <= 0 {
		return d, fmt.Errorf("needMB must be positive, got %d", needMB)
	}

	budget, err := s.BudgetMB()
	if err != nil {
		// Branch: probe unavailable -> deny, and say why. Never silently allow.
		d.Allowed = false
		d.Reason = "hardware probe failed; falling back to CPU"
		return d, err
	}
	d.BudgetMB = budget
	d.FreeMB = budget

	// Branch: enough room right now.
	if needMB <= budget {
		d.Allowed = true
		d.Reason = "budget sufficient"
		return d, nil
	}

	// Branch: need eviction. Evict least-recently-used first; among equally stale
	// entries prefer the one that frees the most memory.
	candidates := s.evictCandidates()
	freed := 0
	for _, c := range candidates {
		d.Evict = append(d.Evict, c.Key)
		freed += c.VRAMMB
		if budget+freed >= needMB {
			d.Allowed = true
			d.FreeMB = budget + freed
			d.Reason = fmt.Sprintf("evict %d model(s) to free %d MB", len(d.Evict), freed)
			return d, nil
		}
	}

	// Branch: eviction cannot satisfy the request.
	d.Allowed = false
	d.Evict = nil // nothing should be touched when the outcome is denial
	d.FreeMB = budget + freed
	d.Reason = fmt.Sprintf("insufficient VRAM: need %d MB, budget %d MB, max reclaimable %d MB",
		needMB, budget, budget+freed)
	return d, nil
}

// evictCandidates returns unpinned residents ordered by LRU, then by size desc.
func (s *Scheduler) evictCandidates() []Entry {
	out := make([]Entry, 0, len(s.resident))
	for _, e := range s.resident {
		if e.Pinned {
			continue
		}
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastUsed.Equal(out[j].LastUsed) {
			return out[i].VRAMMB > out[j].VRAMMB
		}
		return out[i].LastUsed.Before(out[j].LastUsed)
	})
	return out
}

// Load records that a model now occupies VRAM.
func (s *Scheduler) Load(key string, vramMB int) error {
	if key == "" {
		return errors.New("key must not be empty")
	}
	if vramMB < 0 {
		return fmt.Errorf("vramMB must be >= 0, got %d", vramMB)
	}
	s.resident[key] = &Entry{Key: key, VRAMMB: vramMB, LastUsed: s.now()}
	return nil
}

// Touch marks a resident model as recently used, protecting it from eviction.
func (s *Scheduler) Touch(key string) bool {
	e, ok := s.resident[key]
	if !ok {
		return false
	}
	e.LastUsed = s.now()
	return true
}

// Pin protects a model that a running task depends on.
func (s *Scheduler) Pin(key string, pinned bool) bool {
	e, ok := s.resident[key]
	if !ok {
		return false
	}
	e.Pinned = pinned
	return true
}

// Evict removes a model from the residency table.
func (s *Scheduler) Evict(key string) bool {
	if _, ok := s.resident[key]; !ok {
		return false
	}
	delete(s.resident, key)
	return true
}

// Resident returns a snapshot ordered by LRU (oldest first).
func (s *Scheduler) Resident() []Entry {
	out := make([]Entry, 0, len(s.resident))
	for _, e := range s.resident {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsed.Before(out[j].LastUsed) })
	return out
}

// UsedMB is the total VRAM held by resident models.
func (s *Scheduler) UsedMB() int {
	total := 0
	for _, e := range s.resident {
		total += e.VRAMMB
	}
	return total
}

// Backend names an inference backend.
type Backend string

const (
	BackendCUDA12   Backend = "cuda12"
	BackendVulkan   Backend = "vulkan"
	BackendDirectML Backend = "directml"
	BackendCPU      Backend = "cpu"
)

// Hardware is the fact set used to pick a backend.
//
type Hardware struct {
	HasNVIDIA bool
	HasAMD    bool
	HasIntel  bool
	// DriverMajor is the NVIDIA driver major version; meaningless for other vendors,
	// so it is only consulted when HasNVIDIA is set.
	DriverMajor       int
	CUDA12Available   bool
	VulkanAvailable   bool
	DirectMLAvailable bool
}

// HasAnyGPU reports whether a discrete or integrated GPU was detected at all.
func (h Hardware) HasAnyGPU() bool { return h.HasNVIDIA || h.HasAMD || h.HasIntel }

// Vendor names the detected GPU vendor for UI messages.
func (h Hardware) Vendor() string {
	switch {
	case h.HasNVIDIA:
		return "NVIDIA"
	case h.HasAMD:
		return "AMD"
	case h.HasIntel:
		return "Intel"
	default:
		return "none"
	}
}

// Selection is a backend choice plus the reason it was chosen, so the UI can
// show *why* (ADR-004 forbids silent downgrades).
type Selection struct {
	Backend Backend
	Reason  string
}

const minDriverMajor = 525

// SelectBackend implements the fallback chain: cuda12 -> vulkan -> directml -> cpu.
// locked, when non-empty, overrides the chain and is honoured verbatim.
func SelectBackend(hw Hardware, locked Backend) Selection {
	if locked != "" && locked != "auto" {
		return Selection{Backend: locked, Reason: "locked by user setting"}
	}

	// NVIDIA first: CUDA is the fastest path when it is actually present.
	if hw.HasNVIDIA {
		switch {
		case hw.DriverMajor == 0 && hw.CUDA12Available:
			// Driver version unknown but the runtime resolves: try CUDA. Guessing
			// "too old" here would strand a working machine on a slow path.
			return Selection{Backend: BackendCUDA12, Reason: "NVIDIA GPU, CUDA runtime present (driver version unknown)"}
		case hw.DriverMajor >= minDriverMajor && hw.CUDA12Available:
			return Selection{Backend: BackendCUDA12, Reason: "NVIDIA GPU with CUDA 12 runtime"}
		}
	}

	// Every vendor uses Vulkan. This is the branch that keeps AMD and Intel GPUs
	// out of the CPU fallback: the driver ships the loader, so no vendor SDK is
	// needed at runtime.
	if hw.VulkanAvailable && hw.HasAnyGPU() {
		reason := fmt.Sprintf("%s GPU, Vulkan runtime available", hw.Vendor())
		if hw.HasNVIDIA {
			reason = fmt.Sprintf("NVIDIA GPU without usable CUDA (driver %d), Vulkan runtime available", hw.DriverMajor)
		}
		return Selection{Backend: BackendVulkan, Reason: reason}
	}

	// DirectML is deliberately never selected: whisper.cpp has no DirectML backend,
	// so offering it would be a promise the engine cannot keep (ADR-018).
	if hw.DirectMLAvailable && !hw.VulkanAvailable {
		return Selection{Backend: BackendCPU,
			Reason: fmt.Sprintf("%s GPU but no Vulkan loader; DirectML is not supported by the engine, using CPU", hw.Vendor())}
	}

	if hw.HasAnyGPU() {
		return Selection{Backend: BackendCPU,
			Reason: fmt.Sprintf("%s GPU but no Vulkan loader available, using CPU", hw.Vendor())}
	}
	return Selection{Backend: BackendCPU, Reason: "no GPU detected, using CPU"}
}

// DowngradeChain returns the ordered backends to try after from fails.
func DowngradeChain(from Backend) []Backend {
	switch from {
	case BackendCUDA12:
		return []Backend{BackendVulkan, BackendDirectML, BackendCPU}
	case BackendVulkan:
		return []Backend{BackendDirectML, BackendCPU}
	case BackendDirectML:
		return []Backend{BackendCPU}
	default:
		return nil
	}
}

// ModelDowngrade returns the next smaller model id, or "" when already smallest.
func ModelDowngrade(chain []string, current string) string {
	for i, id := range chain {
		if id == current && i+1 < len(chain) {
			return chain[i+1]
		}
	}
	return ""
}
