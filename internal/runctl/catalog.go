package runctl

import (
	"path/filepath"

	"github.com/hhx465453939/stemscribe/internal/engine"
	"github.com/hhx465453939/stemscribe/internal/registry"
)

// CatalogItem is one downloadable thing as the model centre needs it.
//
// SizeMB/VRAMMB come from the registry, not from a guess: they are the numbers
// install pre-flight uses, so the page cannot disagree with the installer.
type CatalogItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	License   string `json:"license"`
	Tier      string `json:"tier,omitempty"`
	SizeMB    int    `json:"size_mb"`
	VRAMMB    int    `json:"vram_mb"`
	Installed bool   `json:"installed"`
	// Version and Path exist so the Models page can answer "where is it and what
	// is it?" without guessing. Path is the install directory, and is only set
	// when the item is actually installed.
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
	// Backend is the device a runtime actually provides (cuda12/cpu for the
	// separation venv). Upgrade marks an installed item whose device is not what
	// this machine should have -- the Python runtime was CPU-only on a GPU box.
	Backend string `json:"backend,omitempty"`
	Upgrade bool   `json:"upgrade,omitempty"`
}

// Catalog is the split the model centre renders.
type Catalog struct {
	Installed []CatalogItem `json:"installed"`
	Available []CatalogItem `json:"available"`
}

// Catalog reports what is installed versus what the registry offers.
//
// Installed means the weight file (or runtime entry point) exists on disk. The page
// used to claim Whisper Medium and an MDX separator were installed while only
// whisper-tiny was; a user acting on that would file a bug against a machine that
// was simply never provisioned.
func CatalogNow(workdir string) (Catalog, error) {
	reg, err := registry.Builtin()
	if err != nil {
		return Catalog{}, err
	}

	out := Catalog{Installed: []CatalogItem{}, Available: []CatalogItem{}}

	for _, m := range reg.Models {
		path, ok := engine.FindModel(workdir, reg, m.ID)
		item := CatalogItem{
			ID:        m.ID,
			Name:      m.DisplayName,
			Kind:      m.Family,
			License:   m.License.SPDX,
			Tier:      m.QualityTier,
			SizeMB:    m.DiskAfterInstallMB,
			VRAMMB:    m.VRAMEstimateMB,
			Installed: ok,
			Version:   m.Quantization,
		}
		if ok {
			item.Path = filepath.Dir(path)
			out.Installed = append(out.Installed, item)
		} else {
			out.Available = append(out.Available, item)
		}
	}

	// The separator runtime is a Python virtualenv, not a registry archive, so it has
	// no entry to iterate over. Without a row here the model centre cannot install it,
	// and a first run on a fresh machine can only be unblocked by dropping to the CLI
	// -- the window would tell the user to run a command it does not offer.
	{
		_, sepOK := engine.FindSeparatorPython(workdir)
		info := engine.ReadSeparatorRuntime(workdir)
		accel := info.Accel
		if accel == "" {
			// Installs predating the accel record are the CPU-only environment.
			accel = "cpu"
		}
		sep := CatalogItem{
			ID:        engine.SeparatorRuntimeID,
			Name:      "UVR5 separator (Python venv)",
			Kind:      "runtime",
			License:   "MIT",
			SizeMB:    separatorRuntimeSizeMB(accel),
			Installed: sepOK,
			Version:   engine.SeparatorVersion(workdir),
			Path:      filepath.Join(workdir, "runtimes", "separator"),
			Backend:   accel,
			// A CPU install on a machine that can do CUDA is exactly the state that
			// made separation hammer the CPU while the GPU idled; offer the upgrade
			// instead of a dead-end "verified".
			Upgrade: separatorNeedsUpgrade(sepOK, accel, SeparatorDesiredAccel()),
		}
		if !sepOK {
			sep.Path = ""
			sep.Upgrade = false
		}
		if sepOK {
			out.Installed = append(out.Installed, sep)
		} else {
			out.Available = append(out.Available, sep)
		}
	}

	for _, r := range reg.Runtimes {
		// Querying by backend is wrong for runtimes: ffmpeg and the whisper cpu build
		// both sit under backend "cpu", so a backend lookup reports one of them as
		// present and the other as missing. ffmpeg already has a precise finder; the
		// rest are matched on product id, not on the backend they happen to share.
		var ok bool
		var root string
		if r.Engine == "ffmpeg" {
			var exe string
			exe, ok = engine.FindFFmpeg(workdir)
			if ok {
				root = filepath.Dir(exe)
			}
		} else if rt, found := engine.FindRuntime(workdir, r.Backend); found {
			ok = true
			root = rt.Root
		}
		item := CatalogItem{
			ID:        r.ID,
			Name:      r.ID,
			Kind:      "runtime",
			License:   r.License.SPDX,
			VRAMMB:    r.VRAMOverheadMB,
			Installed: ok,
			Version:   r.Version,
		}
		if ok {
			item.Path = root
			out.Installed = append(out.Installed, item)
		} else {
			out.Available = append(out.Available, item)
		}
	}

	return out, nil
}

// separatorRuntimeSizeMB is the installed footprint of the Python environment.
//
// The CPU number was measured on a clean venv with audio-separator[cpu]; the CUDA
// number adds onnxruntime-gpu and the nvidia-* wheels that carry cublas/cufft/
// cudnn. Both are shown before install, so both are measured, not estimated.
func separatorRuntimeSizeMB(accel string) int {
	if accel == "cuda12" {
		// Measured 2026-11-26 on the real workdir after upgrading in place:
		// audio-separator + torch-cpu + onnxruntime-gpu + the nvidia cu12 wheels.
		return 3728
	}
	return 1040
}

// separatorNeedsUpgrade reports whether an installed environment runs on a
// different device than this machine wants, i.e. the Models page should offer
// the CUDA upgrade. Not installed means "install", not "upgrade".
func separatorNeedsUpgrade(installed bool, accel, desired string) bool {
	return installed && accel != desired
}
