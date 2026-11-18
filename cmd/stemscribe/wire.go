package main

import (
	"fmt"
	"strings"

	"github.com/hhx465453939/stemscribe/internal/engine"
	"github.com/hhx465453939/stemscribe/internal/pipeline"
	"github.com/hhx465453939/stemscribe/internal/registry"
	"github.com/hhx465453939/stemscribe/internal/resource"
)

// backendFor mirrors what `stemscribe probe` reports, so run and probe can never
// disagree about which backend is in effect.
func backendFor(locked string) (registry.Backend, string) {
	probe := resource.NewProbe()
	hw := resource.Hardware{}
	if total, err := probe.TotalMB(); err == nil && total > 0 {
		hw.HasNVIDIA = resource.LooksNVIDIA()
		hw.DriverMajor = resource.DriverMajor()
		hw.CUDA12Available = resource.CUDA12Present()
	}
	hw.VulkanAvailable = resource.VulkanPresent()

	sel := resource.SelectBackend(hw, resource.Backend(locked))
	// resource and registry name the same backends; convert through string so a
	// typo becomes a compile-time-visible empty map lookup rather than a silent
	// mismatch.
	switch string(sel.Backend) {
	case string(registry.BackendCUDA12):
		return registry.BackendCUDA12, sel.Reason
	case string(registry.BackendVulkan):
		return registry.BackendVulkan, sel.Reason
	default:
		return registry.BackendCPU, sel.Reason
	}
}

// buildEngine assembles the pipeline engine.
//
// "synthetic" stays available on purpose: it is the test double that lets the
// pipeline be exercised without a 78 MB model download, and CHG-0001's smoke test
// depends on it. It is never the default.
func buildEngine(kind, workdir, modelID, locked string, durationMS int) (pipeline.Engine, string, error) {
	if strings.EqualFold(kind, "synthetic") {
		return newSyntheticEngine(durationMS), "synthetic (test double, does not read audio)", nil
	}

	reg, err := registry.Builtin()
	if err != nil {
		return nil, "", fmt.Errorf("load registry: %w", err)
	}

	backend, reason := backendFor(locked)
	missing := engine.Diagnose(workdir, reg, backend, modelID)
	if !missing.Empty() {
		// Surface every gap at once; the caller prints this verbatim.
		return nil, "", missing.Err()
	}

	rt, ok := engine.FindRuntime(workdir, backend)
	if !ok {
		return nil, "", fmt.Errorf("runtime for backend %s vanished after diagnosis", backend)
	}
	modelPath, ok := engine.FindModel(workdir, reg, modelID)
	if !ok {
		return nil, "", fmt.Errorf("model %s vanished after diagnosis", modelID)
	}

	w := &engine.Whisper{
		Exe:       rt.Entry,
		ModelPath: modelPath,
		Backend:   string(backend),
		Version:   rt.Version,
	}
	desc := fmt.Sprintf("%s %s (%s)", "whisper.cpp", rt.Version, backend)
	if reason != "" {
		desc += " · " + reason
	}
	return w, desc, nil
}
