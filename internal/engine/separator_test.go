package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hhx465453939/stemscribe/internal/registry"
)

func TestEnsureSeparatorAssetsWritesHelperAndMetadata(t *testing.T) {
	wd := t.TempDir()
	helper, metaDir, err := EnsureSeparatorAssets(wd)
	if err != nil {
		t.Fatalf("EnsureSeparatorAssets: %v", err)
	}
	if _, err = os.Stat(helper); err != nil {
		t.Fatalf("helper not written: %v", err)
	}
	for _, name := range []string{"download_checks.json", "mdx_model_data.json", "vr_model_data.json"} {
		p := filepath.Join(metaDir, name)
		info, statErr := os.Stat(p)
		if statErr != nil || info.Size() == 0 {
			t.Fatalf("metadata %s missing or empty: %v", name, statErr)
		}
	}
}

func TestEnsureSeparatorAssetsIsIdempotent(t *testing.T) {
	wd := t.TempDir()
	if _, _, err := EnsureSeparatorAssets(wd); err != nil {
		t.Fatalf("first call: %v", err)
	}
	helper := filepath.Join(wd, "runtimes", "separator", "helper.py")
	before, err := os.Stat(helper)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = EnsureSeparatorAssets(wd); err != nil {
		t.Fatalf("second call: %v", err)
	}
	after, err := os.Stat(helper)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("helper rewritten on an unchanged second call (mtime moved)")
	}
}

func TestFindSeparatorPythonHonoursOverride(t *testing.T) {
	wd := t.TempDir()
	fake := filepath.Join(wd, "python.exe")
	if err := os.WriteFile(fake, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEMSCRIBE_SEPARATOR_PYTHON", fake)
	got, ok := FindSeparatorPython(wd)
	if !ok || got != fake {
		t.Fatalf("FindSeparatorPython = %q,%v want %q,true", got, ok, fake)
	}
}

func TestFindSeparatorPythonMissing(t *testing.T) {
	wd := t.TempDir()
	t.Setenv("STEMSCRIBE_SEPARATOR_PYTHON", "")
	if got, ok := FindSeparatorPython(wd); ok {
		t.Fatalf("expected no interpreter, got %q", got)
	}
}

// AC-02: a missing separation dependency must produce an install command, not a
// skipped stage.
func TestMissingHintsNameSeparatorInstall(t *testing.T) {
	m := Missing{SeparatorRuntime: true, SeparatorModels: []string{"uvr-mdx-kim-vocal-2"}}
	if m.Empty() {
		t.Fatal("Missing with separator gaps reported empty")
	}
	text := strings.Join(m.Hints(), "\n")
	if !strings.Contains(text, "runtime install "+SeparatorRuntimeID) {
		t.Fatalf("hints do not name the runtime install command:\n%s", text)
	}
	if !strings.Contains(text, "model install uvr-mdx-kim-vocal-2") {
		t.Fatalf("hints do not name the model install command:\n%s", text)
	}
}

// A hint must never name an install that the installer would refuse: cuda12 has no
// published archive (sha256 tbd), so a CUDA-preferring host must be pointed at
// vulkan instead.
func TestDiagnoseSuggestsOnlyInstallableRuntime(t *testing.T) {
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	wd := t.TempDir()
	m := Diagnose(wd, reg, registry.BackendCUDA12, "whisper-large-v3", true, "uvr-mdx-kim-vocal-2")
	if m.Runtime == "" {
		t.Fatal("no runtime suggested")
	}
	var found *registry.Runtime
	for i := range reg.Runtimes {
		if reg.Runtimes[i].ID == m.Runtime {
			found = &reg.Runtimes[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("suggested runtime %q is not in the registry", m.Runtime)
	}
	if found.Archive.SHA256 == nil || strings.TrimSpace(*found.Archive.SHA256) == "" {
		t.Fatalf("suggested runtime %q has no measured sha256; install would refuse it", m.Runtime)
	}
}

// The separator entry uses zero VRAM (CPU onnxruntime); whisper models still have
// to declare a positive budget.
func TestRegistryAcceptsCPUSeparatorWithoutVRAM(t *testing.T) {
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatalf("builtin registry: %v", err)
	}
	m, ok := reg.FindModel("uvr-mdx-kim-vocal-2")
	if !ok {
		t.Fatal("separation model missing from the catalog")
	}
	if m.Family != string(registry.EngineSeparator) {
		t.Fatalf("family = %q", m.Family)
	}
	if m.Files[0].SHA256 == nil {
		t.Fatal("separation model has no measured sha256")
	}
}

// A multi-file model is installed only when every declared file is present, and
// the file handed to audio-separator is the YAML for Demucs. Checking just the
// first manifest entry used to report a half-copied Demucs directory as ready.
func TestFindModelRequiresWholeManifest(t *testing.T) {
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatalf("builtin registry: %v", err)
	}
	wd := t.TempDir()
	m, ok := reg.FindModel("demucs-htdemucs")
	if !ok || len(m.Files) < 2 {
		t.Fatal("expected a multi-file Demucs entry in the catalog")
	}
	dir := filepath.Join(wd, "models", m.Family, m.ID)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Neither file present.
	if _, ok = FindModel(wd, reg, m.ID); ok {
		t.Fatal("empty directory must not read as installed")
	}

	// Only the weights: the model is still not usable.
	weight := filepath.Join(dir, filepath.FromSlash(m.Files[1].Dest))
	if err = os.WriteFile(weight, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok = FindModel(wd, reg, m.ID); ok {
		t.Fatal("missing YAML must not read as installed")
	}

	// Both files: installed, and the YAML is what the engine loads.
	yamlPath := filepath.Join(dir, filepath.FromSlash(m.Files[0].Dest))
	if err = os.WriteFile(yamlPath, []byte("models: ['x']"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := FindModel(wd, reg, m.ID)
	if !ok {
		t.Fatal("complete Demucs manifest should read as installed")
	}
	if !strings.HasSuffix(got, ".yaml") {
		t.Fatalf("Demucs entry file = %q, want the YAML", filepath.Base(got))
	}
}
