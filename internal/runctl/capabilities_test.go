package runctl

import (
	"testing"

	"github.com/hhx465453939/stemscribe/internal/engine"
)

// Capabilities must name the device from the install record, and say "CPU only"
// when that is what the environment really is. This is the field the user looks
// at after the GPU sat idle through a whole separation.
func TestSeparationBackendStatus(t *testing.T) {
	cases := []struct {
		name       string
		separation bool
		info       engine.SeparatorRuntimeInfo
		wantDev    string
		wantSuffix string
	}{
		{
			name:       "cuda install",
			separation: true,
			info:       engine.SeparatorRuntimeInfo{Accel: "cuda12", Providers: []string{"CUDAExecutionProvider", "CPUExecutionProvider"}},
			wantDev:    "cuda12",
			wantSuffix: " · CUDA",
		},
		{
			name:       "cpu install with reason",
			separation: true,
			info:       engine.SeparatorRuntimeInfo{Accel: "cpu", Reason: "CUDAExecutionProvider is not available"},
			wantDev:    "cpu",
			wantSuffix: " · CPU only (CUDAExecutionProvider is not available)",
		},
		{
			name:       "cpu install without reason",
			separation: true,
			info:       engine.SeparatorRuntimeInfo{Accel: "cpu"},
			wantDev:    "cpu",
			wantSuffix: " · CPU only",
		},
		{
			name:       "legacy install has no record",
			separation: true,
			info:       engine.SeparatorRuntimeInfo{},
			wantDev:    "cpu",
			wantSuffix: " · CPU only",
		},
		{
			name:       "nothing installed",
			separation: false,
			info:       engine.SeparatorRuntimeInfo{},
			wantDev:    "",
			wantSuffix: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dev, suffix := separationBackendStatus(c.separation, c.info)
			if dev != c.wantDev || suffix != c.wantSuffix {
				t.Fatalf("got (%q,%q), want (%q,%q)", dev, suffix, c.wantDev, c.wantSuffix)
			}
		})
	}
}
