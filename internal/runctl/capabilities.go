package runctl

import (
	"github.com/hhx465453939/stemscribe/internal/engine"
	"github.com/hhx465453939/stemscribe/internal/registry"
)

// Capabilities describes what this machine can actually do right now.
//
// The point of UnavailableParams is AC-09: the parameters page must grey out a knob
// with a reason instead of offering a control that will fail at run time. Deciding
// that here -- where the registry and the installed runtimes are both visible -- is
// the only place it can be decided honestly.
type Capabilities struct {
	Backend           string            `json:"backend"`
	BackendReason     string            `json:"backend_reason"`
	GPU               bool              `json:"gpu"`
	Separation        bool              `json:"separation"`
	SeparationReason  string            `json:"separation_reason"`
	Polish            bool              `json:"polish"`
	PolishReason      string            `json:"polish_reason"`
	UnavailableParams map[string]string `json:"unavailable_params"`
}

// CapabilitiesNow probes the machine and the registry.
func CapabilitiesNow(workdir string) (Capabilities, error) {
	backend, reason := BackendFor("", workdir)

	c := Capabilities{
		Backend:           string(backend),
		BackendReason:     reason,
		GPU:               backend != registry.BackendCPU,
		Separation:        false,
		SeparationReason:  "vocal separation is not implemented yet (CHG-0003)",
		Polish:            false,
		PolishReason:      "LLM polishing needs a provider (CHG-0004)",
		UnavailableParams: map[string]string{},
	}

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

	return c, nil
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
