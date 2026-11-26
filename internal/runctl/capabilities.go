package runctl

import (
	"github.com/hhx465453939/stemscribe/internal/engine"
	"github.com/hhx465453939/stemscribe/internal/registry"
	"github.com/hhx465453939/stemscribe/internal/resource"
)

// Capabilities describes what this machine can actually do right now.
//
// The point of UnavailableParams is AC-09: the parameters page must grey out a knob
// with a reason instead of offering a control that will fail at run time. Deciding
// that here -- where the registry and the installed runtimes are both visible -- is
// the only place it can be decided honestly.
type Capabilities struct {
	Backend       string `json:"backend"`
	BackendReason string `json:"backend_reason"`
	// RuntimeVersion is the installed whisper runtime the backend will use. Empty
	// means nothing is installed, which is why the reason says so.
	BackendRuntimeVersion string `json:"backend_runtime_version,omitempty"`
	GPU                   bool   `json:"gpu"`
	// GPUName is measured through nvidia-smi; empty on non-NVIDIA machines. The
	// sidebar shows "VULKAN · RTX 3060" instead of a bare backend id.
	GPUName string `json:"gpu_name,omitempty"`
	// VRAMTotalMB is measured, not asserted: the sidebar used to print a hardcoded
	// "GPU 8.1 / 12 GB" from the dictionary, which disagreed with the same screen's
	// own backend line. 0 means "not measured" and the UI shows a dash.
	VRAMTotalMB       int               `json:"vram_total_mb"`
	VRAMFreeMB        int               `json:"vram_free_mb"`
	Separation        bool              `json:"separation"`
	SeparationReason  string            `json:"separation_reason"`
	Polish            bool              `json:"polish"`
	PolishReason      string            `json:"polish_reason"`
	UnavailableParams map[string]string `json:"unavailable_params"`
}

// CapabilitiesNow probes the machine and the registry.
func CapabilitiesNow(workdir string) (Capabilities, error) {
	bs := BackendReport("", workdir)

	c := Capabilities{
		Backend:               string(bs.Backend),
		BackendReason:         bs.Reason,
		BackendRuntimeVersion: bs.RuntimeVersion,
		GPU:                   bs.Backend != registry.BackendCPU,
		GPUName:               resource.DeviceName(),
		Polish:                false,
		PolishReason:          "LLM polishing needs a provider (CHG-0004)",
		UnavailableParams:     map[string]string{},
	}

	// Separation is implemented (ADR-037); what varies is whether this machine has
	// the Python runtime and the UVR5 weights. The old reason claimed separation was
	// unimplemented, which was a lie the user could see through in one click.
	c.Separation, c.SeparationReason = separationStatus(workdir)

	// Voice activity detection needs a dedicated VAD model file, which the registry
	// does not ship. Offering the switch anyway is how a run ends in an opaque
	// whisper-cli exit 10 instead of an explanation.
	if !vadModelAvailable(workdir) {
		c.UnavailableParams["vad"] = "no VAD model is installed; whisper.cpp requires one for --vad"
	}

	// A device index is meaningless on the CPU build, and selecting one there used to
	// be an accepted-but-ignored setting.
	if !c.GPU {
		c.UnavailableParams["gpu_device"] = "no GPU runtime is installed; this run uses the CPU build"
	}

	// Report measured memory so the resource bar cannot claim a number nobody probed.
	probe := resource.NewProbe()
	if total, err := probe.TotalMB(); err == nil {
		c.VRAMTotalMB = total
	}
	if free, err := probe.FreeMB(); err == nil {
		c.VRAMFreeMB = free
	}

	return c, nil
}

// separationStatus reports whether the mandatory UVR5 stage can actually run.
// Both halves are required: the Python environment and the weights. Naming only
// one would send the user to install something they already have.
func separationStatus(workdir string) (bool, string) {
	settings, _, err := LoadSettings(workdir)
	if err != nil {
		return false, "settings are unreadable: " + err.Error()
	}
	modelID := settings.Stages.Separator
	if modelID == "" {
		modelID = "uvr-mdx-kim-vocal-2"
	}
	_, hasPython := engine.FindSeparatorPython(workdir)
	var hasModel bool
	if reg, regErr := registry.Builtin(); regErr == nil {
		_, hasModel = engine.FindModel(workdir, reg, modelID)
	}
	switch {
	case hasPython && hasModel:
		version := engine.SeparatorVersion(workdir)
		if version == "" {
			version = "installed"
		}
		return true, "UVR5 via audio-separator " + version + " · " + modelID
	case !hasPython && !hasModel:
		return false, "separation runtime and model are not installed (Models page)"
	case !hasPython:
		return false, "separation runtime (audio-separator) is not installed (Models page)"
	default:
		return false, "separation model " + modelID + " is not installed (Models page)"
	}
}

// vadModelAvailable reports whether any installed model looks like a VAD model.
//
// Kept deliberately narrow: the registry has no VAD family yet, so this is false
// today. It exists so that installing one later flips the switch automatically
// instead of leaving a hardcoded "unavailable" behind.
func vadModelAvailable(workdir string) bool {
	reg, err := registry.Builtin()
	if err != nil {
		return false
	}
	for _, m := range reg.Models {
		if m.Family == "vad" {
			if _, ok := engine.FindModel(workdir, reg, m.ID); ok {
				return true
			}
		}
	}
	return false
}
