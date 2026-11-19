package resource

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeProbe lets every branch be reached without a GPU (AC-12).
type fakeProbe struct {
	total    int
	free     int
	totalErr error
	freeErr  error
}

func (f fakeProbe) TotalMB() (int, error) {
	if f.totalErr != nil {
		return 0, f.totalErr
	}
	return f.total, nil
}

func (f fakeProbe) FreeMB() (int, error) {
	if f.freeErr != nil {
		return 0, f.freeErr
	}
	return f.free, nil
}

func TestBudgetSubtractsReserve(t *testing.T) {
	s := New(fakeProbe{total: 12288, free: 11000}, 1536)
	got, err := s.BudgetMB()
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if got != 9464 {
		t.Errorf("budget = %d, want 9464", got)
	}
}

func TestBudgetClampsAtZero(t *testing.T) {
	s := New(fakeProbe{total: 12288, free: 800}, 1536)
	got, err := s.BudgetMB()
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if got != 0 {
		t.Errorf("budget = %d, want 0 when free < reserve", got)
	}
}

func TestBudgetRequiresProbe(t *testing.T) {
	s := New(nil, 1536)
	if _, err := s.BudgetMB(); !errors.Is(err, ErrProbeUnavailable) {
		t.Fatalf("err = %v, want ErrProbeUnavailable", err)
	}
}

func TestBudgetPropagatesProbeErrors(t *testing.T) {
	probeErr := errors.New("nvml init failed")
	for name, p := range map[string]fakeProbe{
		"free":  {freeErr: probeErr},
		"total": {totalErr: probeErr},
	} {
		s := New(p, 1024)
		if _, err := s.BudgetMB(); !errors.Is(err, ErrProbeUnavailable) {
			t.Errorf("%s: err = %v, want ErrProbeUnavailable", name, err)
		}
	}
}

// Branch 1: budget sufficient, no eviction.
func TestAdmitWhenBudgetSufficient(t *testing.T) {
	s := New(fakeProbe{total: 12288, free: 11000}, 1536)
	d, err := s.Admit(2600)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if !d.Allowed {
		t.Errorf("allowed = false, reason %q", d.Reason)
	}
	if len(d.Evict) != 0 {
		t.Errorf("evict = %v, want none", d.Evict)
	}
	if d.BudgetMB != 9464 {
		t.Errorf("budget reported = %d, want 9464", d.BudgetMB)
	}
}

// Branch 2: insufficient, but evicting a stale model makes room.
func TestAdmitEvictsLRUToMakeRoom(t *testing.T) {
	s := New(fakeProbe{total: 12288, free: 3000}, 1536) // budget = 1464
	base := time.Date(2026, 11, 17, 22, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }

	if err := s.Load("whisper:medium", 2600); err != nil {
		t.Fatalf("load: %v", err)
	}
	s.now = func() time.Time { return base.Add(time.Minute) }
	if err := s.Load("separator:mdx", 1500); err != nil {
		t.Fatalf("load: %v", err)
	}

	d, err := s.Admit(3000)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if !d.Allowed {
		t.Fatalf("allowed = false, reason %q", d.Reason)
	}
	if len(d.Evict) == 0 {
		t.Fatal("expected eviction plan")
	}
	if d.Evict[0] != "whisper:medium" {
		t.Errorf("first eviction = %q, want whisper:medium (least recently used)", d.Evict[0])
	}
	if d.FreeMB < 3000 {
		t.Errorf("free after eviction = %d, want >= 3000", d.FreeMB)
	}
}

// Branch 3: even evicting everything cannot satisfy the request.
func TestAdmitDeniesWhenUnreclaimable(t *testing.T) {
	s := New(fakeProbe{total: 12288, free: 2000}, 1536) // budget = 464
	if err := s.Load("tiny", 100); err != nil {
		t.Fatalf("load: %v", err)
	}

	d, err := s.Admit(20000)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if d.Allowed {
		t.Fatal("allowed = true, but the request exceeds total reclaimable VRAM")
	}
	if len(d.Evict) != 0 {
		t.Errorf("evict = %v, want none on denial (never touch state for a rejected plan)", d.Evict)
	}
	if d.Reason == "" {
		t.Error("reason must explain the denial")
	}
}

// Branch 4: probe unavailable -> denial with an actionable reason.
func TestAdmitDeniesWhenProbeFails(t *testing.T) {
	s := New(nil, 1536)
	d, err := s.Admit(1000)
	if err == nil {
		t.Fatal("expected probe error to be returned")
	}
	if d.Allowed {
		t.Fatal("allowed = true on a CPU-only build")
	}
	if d.Reason == "" {
		t.Error("reason must tell the user we are falling back to CPU")
	}
}

func TestAdmitRejectsNonPositive(t *testing.T) {
	s := New(fakeProbe{total: 12288, free: 11000}, 1536)
	for _, need := range []int{0, -5} {
		if _, err := s.Admit(need); err == nil {
			t.Errorf("Admit(%d) = nil error, want rejection", need)
		}
	}
}

func TestPinnedNeverEvicted(t *testing.T) {
	s := New(fakeProbe{total: 12288, free: 1000}, 1536) // budget 0
	if err := s.Load("running-task", 4000); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !s.Pin("running-task", true) {
		t.Fatal("pin failed")
	}

	d, err := s.Admit(1000)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if d.Allowed {
		t.Fatal("allowed = true, but the only resident model is pinned")
	}
	for _, k := range d.Evict {
		if k == "running-task" {
			t.Fatal("pinned model appeared in the eviction plan")
		}
	}
}

func TestEvictLargerOnEqualStaleness(t *testing.T) {
	s := New(fakeProbe{total: 12288, free: 1000}, 1536)
	fixed := time.Date(2026, 11, 17, 22, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }

	if err := s.Load("small", 500); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Load("large", 4000); err != nil {
		t.Fatalf("load: %v", err)
	}

	d, err := s.Admit(3000)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if len(d.Evict) == 0 || d.Evict[0] != "large" {
		t.Errorf("evict = %v, want large first (ties broken by size)", d.Evict)
	}
}

func TestLoadTouchPinEvict(t *testing.T) {
	s := New(fakeProbe{total: 8192, free: 8192}, 0)

	if err := s.Load("", 100); err == nil {
		t.Error("empty key must be rejected")
	}
	if err := s.Load("m", -1); err == nil {
		t.Error("negative vram must be rejected")
	}
	if err := s.Load("m", 100); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !s.Touch("m") {
		t.Error("Touch should find a resident model")
	}
	if s.Touch("absent") {
		t.Error("Touch should report miss for unknown model")
	}
	if !s.Pin("m", true) {
		t.Error("Pin should find a resident model")
	}
	if s.Pin("absent", true) {
		t.Error("Pin should report miss for unknown model")
	}
	if s.UsedMB() != 100 {
		t.Errorf("used = %d, want 100", s.UsedMB())
	}
	if got := len(s.Resident()); got != 1 {
		t.Errorf("resident count = %d, want 1", got)
	}
	if !s.Evict("m") {
		t.Error("Evict should remove a resident model")
	}
	if s.Evict("m") {
		t.Error("second Evict should report miss")
	}
	if s.UsedMB() != 0 {
		t.Errorf("used after evict = %d, want 0", s.UsedMB())
	}
}

func TestResidentOrderedByLRU(t *testing.T) {
	s := New(fakeProbe{total: 8192, free: 8192}, 0)
	base := time.Date(2026, 11, 17, 22, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }
	if err := s.Load("old", 100); err != nil {
		t.Fatalf("load: %v", err)
	}
	s.now = func() time.Time { return base.Add(time.Hour) }
	if err := s.Load("new", 100); err != nil {
		t.Fatalf("load: %v", err)
	}

	got := s.Resident()
	if len(got) != 2 || got[0].Key != "old" {
		t.Fatalf("Resident() = %v, want oldest first", got)
	}
}

func TestReserveNegativeClamped(t *testing.T) {
	s := New(fakeProbe{total: 1000, free: 1000}, -50)
	if s.ReserveMB() != 0 {
		t.Errorf("reserve = %d, want 0", s.ReserveMB())
	}
}

func TestSelectBackendPrefersCUDA(t *testing.T) {
	got := SelectBackend(Hardware{HasNVIDIA: true, DriverMajor: 566, CUDA12Available: true}, "")
	if got.Backend != BackendCUDA12 {
		t.Errorf("backend = %q, want cuda12", got.Backend)
	}
	if got.Reason == "" {
		t.Error("reason must be populated for the UI")
	}
}

func TestSelectBackendHonoursLock(t *testing.T) {
	got := SelectBackend(Hardware{HasNVIDIA: true, DriverMajor: 566, CUDA12Available: true}, BackendCPU)
	if got.Backend != BackendCPU {
		t.Errorf("backend = %q, want cpu (locked)", got.Backend)
	}
}

func TestSelectBackendOldDriver(t *testing.T) {
	got := SelectBackend(Hardware{HasNVIDIA: true, DriverMajor: 470, VulkanAvailable: true}, "")
	if got.Backend != BackendVulkan {
		t.Errorf("backend = %q, want vulkan", got.Backend)
	}

	// DirectML is never selected: whisper.cpp has no such backend, so a
	// DirectML-only machine must fall to CPU instead of pretending (ADR-018).
	got = SelectBackend(Hardware{HasNVIDIA: true, DriverMajor: 470, DirectMLAvailable: true}, "")
	if got.Backend != BackendCPU {
		t.Errorf("backend = %q, want cpu (DirectML is not supported by the engine)", got.Backend)
	}

	got = SelectBackend(Hardware{HasNVIDIA: true, DriverMajor: 470}, "")
	if got.Backend != BackendCPU {
		t.Errorf("backend = %q, want cpu", got.Backend)
	}
}

func TestSelectBackendUnknownDriver(t *testing.T) {
	// Driver unknown but the runtime resolves: CUDA is still worth trying.
	got := SelectBackend(Hardware{HasNVIDIA: true, DriverMajor: 0, CUDA12Available: true}, "")
	if got.Backend != BackendCUDA12 {
		t.Errorf("backend = %q, want cuda12", got.Backend)
	}
	// Driver unknown and no CUDA runtime: prefer Vulkan over a blind CUDA attempt.
	got = SelectBackend(Hardware{HasNVIDIA: true, DriverMajor: 0, VulkanAvailable: true}, "")
	if got.Backend != BackendVulkan {
		t.Errorf("backend = %q, want vulkan", got.Backend)
	}
}

// TestSelectBackendAMD is the case the owner called the most important: an AMD
// card has to reach the GPU through Vulkan instead of dropping to CPU.
func TestSelectBackendAMD(t *testing.T) {
	got := SelectBackend(Hardware{HasAMD: true, VulkanAvailable: true}, "")
	if got.Backend != BackendVulkan {
		t.Fatalf("AMD GPU must use Vulkan, got %q", got.Backend)
	}
	if !strings.Contains(got.Reason, "AMD") {
		t.Errorf("reason must name the vendor so the UI can explain it, got %q", got.Reason)
	}
}

func TestSelectBackendIntel(t *testing.T) {
	got := SelectBackend(Hardware{HasIntel: true, VulkanAvailable: true}, "")
	if got.Backend != BackendVulkan {
		t.Fatalf("Intel GPU must use Vulkan, got %q", got.Backend)
	}
}

// TestSelectBackendNoVulkanLoader covers a GPU the engine cannot use: say so
// instead of silently running on CPU.
func TestSelectBackendNoVulkanLoader(t *testing.T) {
	got := SelectBackend(Hardware{HasAMD: true}, "")
	if got.Backend != BackendCPU {
		t.Fatalf("backend = %q, want cpu", got.Backend)
	}
	if !strings.Contains(got.Reason, "Vulkan") {
		t.Errorf("reason must mention the missing Vulkan loader, got %q", got.Reason)
	}
}

func TestSelectBackendNoGPU(t *testing.T) {
	got := SelectBackend(Hardware{}, "")
	if got.Backend != BackendCPU {
		t.Errorf("backend = %q, want cpu", got.Backend)
	}
	if !strings.Contains(got.Reason, "no GPU") {
		t.Errorf("reason = %q, want it to say no GPU was found", got.Reason)
	}
}

// TestVendorName keeps the UI-facing string honest.
func TestVendorName(t *testing.T) {
	cases := []struct {
		hw   Hardware
		want string
	}{
		{Hardware{HasNVIDIA: true}, "NVIDIA"},
		{Hardware{HasAMD: true}, "AMD"},
		{Hardware{HasIntel: true}, "Intel"},
		{Hardware{}, "none"},
	}
	for _, c := range cases {
		if got := c.hw.Vendor(); got != c.want {
			t.Errorf("Vendor() = %q, want %q", got, c.want)
		}
	}
}

func TestDowngradeChain(t *testing.T) {
	cases := map[Backend][]Backend{
		BackendCUDA12:   {BackendVulkan, BackendDirectML, BackendCPU},
		BackendVulkan:   {BackendDirectML, BackendCPU},
		BackendDirectML: {BackendCPU},
		BackendCPU:      nil,
	}
	for from, want := range cases {
		got := DowngradeChain(from)
		if len(got) != len(want) {
			t.Errorf("%s: chain = %v, want %v", from, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: chain[%d] = %q, want %q", from, i, got[i], want[i])
			}
		}
	}
}

func TestModelDowngrade(t *testing.T) {
	chain := []string{"large", "medium", "small"}
	if got := ModelDowngrade(chain, "large"); got != "medium" {
		t.Errorf("got %q, want medium", got)
	}
	if got := ModelDowngrade(chain, "small"); got != "" {
		t.Errorf("got %q, want empty at the end of the chain", got)
	}
	if got := ModelDowngrade(chain, "unknown"); got != "" {
		t.Errorf("got %q, want empty for unknown model", got)
	}
}
