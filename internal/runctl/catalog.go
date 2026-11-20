package runctl

import (
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
		_, ok := engine.FindModel(workdir, reg, m.ID)
		item := CatalogItem{
			ID:        m.ID,
			Name:      m.DisplayName,
			Kind:      m.Family,
			License:   m.License.SPDX,
			Tier:      m.QualityTier,
			SizeMB:    m.DiskAfterInstallMB,
			VRAMMB:    m.VRAMEstimateMB,
			Installed: ok,
		}
		if ok {
			out.Installed = append(out.Installed, item)
		} else {
			out.Available = append(out.Available, item)
		}
	}

	for _, r := range reg.Runtimes {
		// Querying by backend is wrong for runtimes: ffmpeg and the whisper cpu build
		// both sit under backend "cpu", so a backend lookup reports one of them as
		// present and the other as missing. ffmpeg already has a precise finder; the
		// rest are matched on product id, not on the backend they happen to share.
		var ok bool
		if r.Engine == "ffmpeg" {
			_, ok = engine.FindFFmpeg(workdir)
		} else {
			// After ffmpeg is handled by its own finder, every remaining backend belongs
			// to exactly one whisper build, so a backend lookup is unambiguous.
			_, ok = engine.FindRuntime(workdir, r.Backend)
		}
		item := CatalogItem{
			ID:        r.ID,
			Name:      r.ID,
			Kind:      "runtime",
			License:   r.License.SPDX,
			VRAMMB:    r.VRAMOverheadMB,
			Installed: ok,
		}
		if ok {
			out.Installed = append(out.Installed, item)
		} else {
			out.Available = append(out.Available, item)
		}
	}

	return out, nil
}
