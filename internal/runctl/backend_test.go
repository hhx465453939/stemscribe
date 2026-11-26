package runctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeRuntime drops a whisper-cli.exe where FindRuntime looks for it, so backend
// selection can be tested without a GPU or a real build.
func makeRuntime(t *testing.T, workdir, versionBackend string) {
	t.Helper()
	dir := filepath.Join(workdir, "runtimes", "whisper", versionBackend)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "whisper-cli.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestBackendReportUsesInstalledRuntime(t *testing.T) {
	wd := t.TempDir()
	makeRuntime(t, wd, "1.9.5-vulkan")

	st := BackendReport("vulkan", wd)
	if st.Backend != "vulkan" {
		t.Fatalf("backend = %q want vulkan", st.Backend)
	}
	if !st.Installed || st.RuntimeVersion != "1.9.5" {
		t.Fatalf("installed=%v version=%q want true/1.9.5", st.Installed, st.RuntimeVersion)
	}
}

// A machine whose hardware prefers cuda12 but only has the vulkan build must run
// vulkan AND say why; the old probe printed cuda12 and the run then used vulkan.
func TestBackendReportNamesTheDowngrade(t *testing.T) {
	wd := t.TempDir()
	makeRuntime(t, wd, "1.9.5-vulkan")

	st := BackendReport("cuda12", wd)
	if st.Backend != "vulkan" {
		t.Fatalf("backend = %q want vulkan", st.Backend)
	}
	if !strings.Contains(st.Reason, "cuda12") || !strings.Contains(st.Reason, "not installed") {
		t.Fatalf("reason %q must name cuda12 and say it is not installed", st.Reason)
	}
}

func TestBackendReportWithoutRuntimeSaysSo(t *testing.T) {
	st := BackendReport("cuda12", t.TempDir())
	if st.Installed {
		t.Fatal("empty workdir must not report an installed runtime")
	}
	if !strings.Contains(st.Reason, "no whisper runtime is installed") {
		t.Fatalf("reason %q must say nothing is installed", st.Reason)
	}
}

// The capabilities page used to claim separation was unimplemented. It is
// implemented; what varies is the installed set, and the reason must say which
// half is missing.
func TestCapabilitiesSeparationReasonIsHonest(t *testing.T) {
	caps, err := CapabilitiesNow(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if caps.Separation {
		t.Fatal("empty workdir must not report separation as available")
	}
	if strings.Contains(caps.SeparationReason, "not implemented") {
		t.Fatalf("stale reason: %q", caps.SeparationReason)
	}
	if !strings.Contains(caps.SeparationReason, "not installed") {
		t.Fatalf("reason %q must name the missing piece", caps.SeparationReason)
	}
}

// probe, /api/capabilities and a run all resolve through BackendReport; this pins
// the shared value so a future edit cannot quietly reintroduce a second opinion.
func TestCapabilitiesBackendMatchesBackendReport(t *testing.T) {
	wd := t.TempDir()
	makeRuntime(t, wd, "1.9.5-vulkan")
	caps, err := CapabilitiesNow(wd)
	if err != nil {
		t.Fatal(err)
	}
	st := BackendReport("", wd)
	if caps.Backend != string(st.Backend) {
		t.Fatalf("capabilities backend %q != BackendReport %q", caps.Backend, st.Backend)
	}
	if caps.BackendRuntimeVersion != st.RuntimeVersion {
		t.Fatalf("runtime version %q != %q", caps.BackendRuntimeVersion, st.RuntimeVersion)
	}
}
