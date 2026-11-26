package registry

import (
	"strings"
	"testing"
)

func TestBuiltinCatalogIsValid(t *testing.T) {
	r, err := Builtin()
	if err != nil {
		t.Fatalf("builtin catalog must validate: %v", err)
	}
	if len(r.Models) < 3 {
		t.Fatalf("expected at least 3 models, got %d", len(r.Models))
	}
	if len(r.Runtimes) < 2 {
		t.Fatalf("expected at least 2 runtimes, got %d", len(r.Runtimes))
	}
}

func TestBuiltinCoversOnboardingTiers(t *testing.T) {
	r, err := Builtin()
	if err != nil {
		t.Fatalf("builtin: %v", err)
	}
	tiers := map[string]bool{}
	for _, m := range r.Models {
		if m.Engine == EngineWhisper {
			tiers[m.QualityTier] = true
		}
	}
	for _, want := range []string{"light", "balanced", "max"} {
		if !tiers[want] {
			t.Errorf("no whisper model for tier %q; onboarding presets would be incomplete", want)
		}
	}
}

func TestBuiltinHasCPUPath(t *testing.T) {
	r, err := Builtin()
	if err != nil {
		t.Fatalf("builtin: %v", err)
	}
	if _, ok := r.FindRuntime(EngineWhisper, BackendCPU, "windows-x64"); !ok {
		t.Fatal("no CPU whisper runtime: the no-GPU path (ADR-004) would be impossible")
	}
}

func TestValidateRejectsTraversalInDest(t *testing.T) {
	raw := baseCatalog(`"dest":"../../evil.bin"`)
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected rejection of traversal dest")
	}
}

func TestValidateRejectsAbsoluteDest(t *testing.T) {
	for _, dest := range []string{`"/etc/passwd"`, `"C:\\Windows\\x.bin"`} {
		raw := baseCatalog(`"dest":` + dest)
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("expected rejection of absolute dest %s", dest)
		}
	}
}

func TestValidateRejectsNullSHAWithoutTBD(t *testing.T) {
	raw := baseCatalog(`"sha256":null,"dest":"m.bin"`)
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("null sha256 without tbd must be rejected: unmeasured checksums are never silently accepted")
	}
}

func TestValidateAcceptsNullSHAWithTBD(t *testing.T) {
	raw := baseCatalog(`"sha256":null,"tbd":true,"dest":"m.bin"`)
	if _, err := Parse([]byte(raw)); err != nil {
		t.Fatalf("null sha256 with tbd should be accepted: %v", err)
	}
}

func TestValidateRejectsBadSHAFormat(t *testing.T) {
	raw := baseCatalog(`"sha256":"DEADBEEF","dest":"m.bin"`)
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("malformed sha256 must be rejected")
	}
}

func TestValidateRejectsUnknownBackend(t *testing.T) {
	raw := strings.Replace(baseCatalog(`"dest":"m.bin","sha256":null,"tbd":true`),
		`"backend":"cpu"`, `"backend":"opencl"`, 1)
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("unknown backend must be rejected")
	}
}

func TestValidateRejectsMissingLicense(t *testing.T) {
	raw := strings.Replace(baseCatalog(`"dest":"m.bin","sha256":null,"tbd":true`),
		`"license":{"spdx":"MIT"}`, `"license":{"spdx":""}`, 1)
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("missing license must be rejected (redistribution compliance)")
	}
}

func TestFindModel(t *testing.T) {
	r, err := Builtin()
	if err != nil {
		t.Fatalf("builtin: %v", err)
	}
	m, ok := r.FindModel("whisper-medium")
	if !ok {
		t.Fatal("whisper-medium not found")
	}
	if m.VRAMEstimateMB <= 0 {
		t.Error("vram estimate must be positive for admission control")
	}
	if _, ok = r.FindModel("does-not-exist"); ok {
		t.Error("unknown id should not resolve")
	}
}

func TestModelsForFiltersByEngine(t *testing.T) {
	r, err := Builtin()
	if err != nil {
		t.Fatalf("builtin: %v", err)
	}
	whisper := r.ModelsFor(EngineWhisper)
	if len(whisper) == 0 {
		t.Fatal("no whisper models")
	}
	for _, m := range whisper {
		if m.Engine != EngineWhisper {
			t.Errorf("model %s has engine %s, want whisper", m.ID, m.Engine)
		}
	}
}

func TestResolveURL(t *testing.T) {
	r, err := Builtin()
	if err != nil {
		t.Fatalf("builtin: %v", err)
	}
	mirror, ok := r.MirrorByID("official")
	if !ok {
		t.Fatal("official mirror missing")
	}

	got := r.ResolveURL(mirror, "runtimes/whisper/1.7.4/win-x64-cpu.zip")
	want := "https://github.com/hhx465453939/stemscribe/releases/download/runtimes/whisper/1.7.4/win-x64-cpu.zip"
	if got != want {
		t.Errorf("ResolveURL = %q, want %q", got, want)
	}

	// Absolute URLs pass through untouched so a mirror can point at a CDN.
	abs := "https://cdn.example.org/x.zip"
	if got = r.ResolveURL(mirror, abs); got != abs {
		t.Errorf("absolute URL rewritten to %q", got)
	}
}

// baseCatalog builds a minimal valid catalog with one override applied to the
// single model file entry, so each negative test changes exactly one field.
func baseCatalog(fileOverride string) string {
	return `{
	  "schema":1,
	  "generated_at":"2026-11-17T00:00:00Z",
	  "mirrors":[{"id":"official","base_url":"https://example.org/dl","priority":0}],
	  "runtimes":[{
	    "id":"rt-1","engine":"whisper","version":"1.0.0","backend":"cpu","platform":"windows-x64",
	    "archive":{"url":"a.zip","size":10,"sha256":null,"tbd":true,"dest":"a.zip"},
	    "entrypoint":"whisper-cli.exe","vram_overhead_mb":0,
	    "license":{"spdx":"MIT"}
	  }],
	  "models":[{
	    "id":"m-1","family":"whisper","engine":"whisper","display_name":"M1",
	    "files":[{"url":"m.bin","size":10,` + fileOverride + `}],
	    "vram_estimate_mb":100,"disk_after_install_mb":10,"quality_tier":"balanced",
	    "license":{"spdx":"MIT"},"source":"https://example.org"
	  }]
	}`
}

func TestUsesTorch(t *testing.T) {
	cases := map[Architecture]bool{
		ArchitectureMDX:      false,
		ArchitectureVR:       true,
		ArchitectureDemucs:   true,
		ArchitectureMDXC:     true,
		ArchitectureRoformer: true,
		"":                   false,
	}
	for arch, want := range cases {
		if got := arch.UsesTorch(); got != want {
			t.Errorf("%q.UsesTorch() = %v, want %v", arch, got, want)
		}
	}
}

func TestValidateSeparatorArchitecture(t *testing.T) {
	th := `{"url":"https://example.org/a.th","size":10,"sha256":null,"tbd":true,"dest":"a.th"}`
	yaml := `{"url":"https://example.org/m.yaml","size":10,"sha256":null,"tbd":true,"dest":"m.yaml"}`
	onnx := `{"url":"https://example.org/m.onnx","size":10,"sha256":null,"tbd":true,"dest":"m.onnx"}`
	cases := []struct {
		name    string
		arch    string
		files   []string
		wantErr bool
	}{
		{"demucs yaml+th ok", "demucs", []string{yaml, th}, false},
		{"demucs onnx rejected", "demucs", []string{onnx}, true},
		{"demucs missing yaml rejected", "demucs", []string{th}, true},
		{"mdx onnx ok", "mdx", []string{onnx}, false},
		{"mdx yaml rejected", "mdx", []string{yaml}, true},
		{"missing architecture rejected", "", []string{onnx}, true},
		{"unknown architecture rejected", "spleeter", []string{onnx}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := separatorCatalog(tc.arch, strings.Join(tc.files, ","), `"vocals","instrumental"`)
			_, err := Parse([]byte(raw))
			if tc.wantErr && err == nil {
				t.Fatal("expected rejection")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected rejection: %v", err)
			}
		})
	}
}

func TestValidateDemucsSixStems(t *testing.T) {
	th := `{"url":"https://example.org/a.th","size":10,"sha256":null,"tbd":true,"dest":"a.th"}`
	yaml := `{"url":"https://example.org/m.yaml","size":10,"sha256":null,"tbd":true,"dest":"m.yaml"}`
	raw := separatorCatalog("demucs", yaml+","+th,
		`"vocals","instrumental","drums","bass","other","guitar","piano"`)
	if _, err := Parse([]byte(raw)); err != nil {
		t.Fatalf("six-stem demucs must validate: %v", err)
	}
}

// separatorCatalog builds a minimal valid catalog whose single model is a
// separator model with the given architecture, files and stems.
func separatorCatalog(arch, files, stems string) string {
	return `{
	  "schema":1,
	  "generated_at":"2026-11-26T00:00:00Z",
	  "mirrors":[{"id":"official","base_url":"https://example.org/dl","priority":0}],
	  "runtimes":[{
	    "id":"rt-1","engine":"whisper","version":"1.0.0","backend":"cpu","platform":"windows-x64",
	    "archive":{"url":"a.zip","size":10,"sha256":null,"tbd":true,"dest":"a.zip"},
	    "entrypoint":"whisper-cli.exe","vram_overhead_mb":0,
	    "license":{"spdx":"MIT"}
	  }],
	  "models":[{
	    "id":"m-1","family":"separator","engine":"separator","display_name":"M1",
	    "architecture":"` + arch + `",
	    "files":[` + files + `],
	    "stems":[` + stems + `],
	    "vram_estimate_mb":100,"ram_estimate_mb":100,"disk_after_install_mb":10,
	    "quality_tier":"balanced",
	    "license":{"spdx":"MIT"},"source":"https://example.org"
	  }]
	}`
}
