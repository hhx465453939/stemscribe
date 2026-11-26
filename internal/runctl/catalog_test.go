package runctl

import (
	"os"
	"path/filepath"
	"testing"
)

// The separator runtime is not a registry entry, so it only reaches the Models page
// through a synthetic row. Without it a fresh machine can be told "install
// separator-python" and have nowhere in the window to do it.
func TestCatalogOffersSeparatorRuntime(t *testing.T) {
	wd := t.TempDir()
	t.Setenv("STEMSCRIBE_SEPARATOR_PYTHON", "")

	cat, err := CatalogNow(wd)
	if err != nil {
		t.Fatalf("CatalogNow: %v", err)
	}
	if findItem(cat.Available, "separator-python") == nil {
		t.Fatal("missing separator runtime is not offered as installable")
	}
	if findItem(cat.Installed, "separator-python") != nil {
		t.Fatal("separator runtime reported installed on an empty workdir")
	}

	// A seeded interpreter flips the row to installed.
	python := filepath.Join(wd, "python.exe")
	if writeErr := os.WriteFile(python, []byte("x"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	t.Setenv("STEMSCRIBE_SEPARATOR_PYTHON", python)
	cat, err = CatalogNow(wd)
	if err != nil {
		t.Fatalf("CatalogNow after seeding: %v", err)
	}
	if findItem(cat.Installed, "separator-python") == nil {
		t.Fatal("seed interpreter not reflected as installed")
	}
}

func TestCatalogInstalledRuntimeExposesPathAndVersion(t *testing.T) {
	wd := t.TempDir()
	makeRuntime(t, wd, "1.9.5-vulkan")

	cat, err := CatalogNow(wd)
	if err != nil {
		t.Fatal(err)
	}
	item := findItem(cat.Installed, "whispercpp-vulkan-win-x64")
	if item == nil {
		t.Fatal("installed vulkan runtime missing from catalog")
	}
	if item.Version != "1.9.5" {
		t.Fatalf("version = %q want 1.9.5", item.Version)
	}
	want := filepath.Join(wd, "runtimes", "whisper", "1.9.5-vulkan")
	if item.Path != want {
		t.Fatalf("path = %q want %q", item.Path, want)
	}
}

func findItem(items []CatalogItem, id string) *CatalogItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}
