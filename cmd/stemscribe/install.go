package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/downloader"
	"github.com/hhx465453939/stemscribe/internal/registry"
)

// verifiedFile records what we installed and where it came from, so provenance can
// be reconstructed even if the registry later changes its checksums.
type verifiedFile struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"` // runtime | model
	Version     string    `json:"version,omitempty"`
	Backend     string    `json:"backend,omitempty"`
	SHA256      string    `json:"sha256"`
	SourceURL   string    `json:"source_url"`
	InstalledAt time.Time `json:"installed_at"`
}

// cmdRuntime handles "runtime install <id>".
func cmdRuntime(args []string) int {
	if len(args) < 2 || args[0] != "install" {
		fmt.Fprintln(os.Stderr, "usage: stemscribe runtime install <runtime-id>")
		return 2
	}
	return installOne(args[1], "runtime", args[2:])
}

// cmdModel handles "model install <id>".
func cmdModel(args []string) int {
	if len(args) < 2 || args[0] != "install" {
		fmt.Fprintln(os.Stderr, "usage: stemscribe model install <model-id>")
		return 2
	}
	return installOne(args[1], "model", args[2:])
}

func installOne(id, kind string, rest []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	workdir := fs.String("workdir", "", "work directory (default: workdir next to the executable)")
	// Parse only what follows the id: os.Args would also swallow the subcommand
	// and the id itself, which silently disabled --workdir.
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintf(os.Stderr, "install: %v\n", err)
		return 2
	}

	wd := *workdir
	if wd == "" {
		var err error
		if wd, err = config.DefaultWorkdir(); err != nil {
			fmt.Fprintf(os.Stderr, "install: %v\n", err)
			return 1
		}
	}
	layout, err := config.NewLayout(wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "install: %v\n", err)
		return 1
	}
	if err = layout.Ensure(); err != nil {
		fmt.Fprintf(os.Stderr, "install: %v\n", err)
		return 1
	}

	reg, err := registry.Builtin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "install: load registry: %v\n", err)
		return 1
	}

	switch kind {
	case "runtime":
		rt, ok := findRuntime(reg, id)
		if !ok {
			fmt.Fprintf(os.Stderr, "install: unknown runtime %q\n", id)
			return 2
		}
		return installRuntime(layout, reg, rt)
	default:
		m, ok := reg.FindModel(id)
		if !ok {
			fmt.Fprintf(os.Stderr, "install: unknown model %q\n", id)
			return 2
		}
		return installModel(layout, reg, m)
	}
}

func findRuntime(reg registry.Registry, id string) (registry.Runtime, bool) {
	for _, rt := range reg.Runtimes {
		if rt.ID == id {
			return rt, true
		}
	}
	return registry.Runtime{}, false
}

// requireChecksum refuses to install an artifact whose checksum has not been
// measured yet. Silently installing an unverified binary is exactly the supply
// chain hole ADR-013 exists to close. This also implements PRD F-10 at the
// command level: an unpublished entry cannot be installed.
func requireChecksum(id string, sha *string) error {
	if sha == nil || strings.TrimSpace(*sha) == "" {
		return fmt.Errorf("%s is not published yet (no measured sha256); refusing to install unverified bytes", id)
	}
	return nil
}

// runtimeDirName builds the install directory for a runtime.
//
// The backend suffix only makes sense when the engine has backends: ffmpeg does
// not, and "N-129046-none" is a name that shows up directly in the user's work
// directory, so it should not contain a meaningless word.
func runtimeDirName(version string, backend registry.Backend) string {
	if backend == "" || backend == registry.BackendNone {
		return version
	}
	return version + "-" + string(backend)
}

func installRuntime(layout config.Layout, reg registry.Registry, rt registry.Runtime) int {
	if err := requireChecksum(rt.ID, rt.Archive.SHA256); err != nil {
		fmt.Fprintf(os.Stderr, "install: %v\n", err)
		return 1
	}
	mirror, ok := reg.MirrorByID("official")
	if !ok {
		fmt.Fprintln(os.Stderr, "install: registry has no official mirror")
		return 1
	}

	url := reg.ResolveURL(mirror, rt.Archive.URL)
	tmpZip := filepath.Join(layout.Tmp(), rt.Archive.Dest)

	fmt.Printf("installing runtime %s %s (%s)\n", rt.ID, rt.Version, rt.Backend)
	fmt.Printf("  from %s\n", url)

	if code := fetch(url, tmpZip, rt.Archive.Size, *rt.Archive.SHA256); code != 0 {
		return code
	}

	destDir := filepath.Join(layout.Runtimes(), string(rt.Engine), runtimeDirName(rt.Version, rt.Backend))
	if err := unzipInto(tmpZip, destDir); err != nil {
		fmt.Fprintf(os.Stderr, "install: extract: %v\n", err)
		return 1
	}
	_ = os.Remove(tmpZip)

	entry := filepath.Join(destDir, filepath.FromSlash(rt.Entrypoint))
	if _, err := os.Stat(entry); err != nil {
		fmt.Fprintf(os.Stderr, "install: archive did not contain %s: %v\n", rt.Entrypoint, err)
		return 1
	}

	if err := writeVerified(layout, verifiedFile{
		ID: rt.ID, Kind: "runtime", Version: rt.Version, Backend: string(rt.Backend),
		SHA256: *rt.Archive.SHA256, SourceURL: url, InstalledAt: time.Now().UTC(),
	}, destDir); err != nil {
		fmt.Fprintf(os.Stderr, "install: record: %v\n", err)
		return 1
	}

	fmt.Printf("installed to %s\n", destDir)
	return 0
}

func installModel(layout config.Layout, reg registry.Registry, m registry.Model) int {
	for _, f := range m.Files {
		if err := requireChecksum(m.ID, f.SHA256); err != nil {
			fmt.Fprintf(os.Stderr, "install: %v\n", err)
			return 1
		}
	}
	mirror, ok := reg.MirrorByID("official")
	if !ok {
		fmt.Fprintln(os.Stderr, "install: registry has no official mirror")
		return 1
	}

	destDir := filepath.Join(layout.Models(), m.Family, m.ID)
	for _, f := range m.Files {
		url := reg.ResolveURL(mirror, f.URL)
		dest := filepath.Join(destDir, filepath.FromSlash(f.Dest))
		fmt.Printf("installing model %s\n  from %s\n", m.ID, url)
		if code := fetch(url, dest, f.Size, *f.SHA256); code != 0 {
			return code
		}
	}

	if err := writeVerified(layout, verifiedFile{
		ID: m.ID, Kind: "model", SHA256: *m.Files[0].SHA256,
		SourceURL: reg.ResolveURL(mirror, m.Files[0].URL), InstalledAt: time.Now().UTC(),
	}, destDir); err != nil {
		fmt.Fprintf(os.Stderr, "install: record: %v\n", err)
		return 1
	}
	fmt.Printf("installed to %s\n", destDir)
	return 0
}

// fetch downloads and verifies one artifact, reporting progress in place.
func fetch(url, dest string, size int64, sha string) int {
	client := &downloader.Client{}
	// Report at 5% steps. The downloader fires every 512 KB, which on a 1.5 GB
	// model is thousands of lines -- unusable in a CI log.
	lastPct := -5
	res, err := client.Fetch(context.Background(), downloader.Artifact{
		URL: url, DestPath: dest, Size: size, SHA256: sha,
	}, func(done, total, bps int64) {
		if total <= 0 {
			return
		}
		pct := int(done * 100 / total)
		if pct-lastPct < 5 && pct != 100 {
			return
		}
		lastPct = pct
		fmt.Printf("  %3d%%  %d/%d MB\n", pct, done>>20, total>>20)
	})
	fmt.Println()
	if err != nil {
		fmt.Fprintf(os.Stderr, "install: %v\n", err)
		return 1
	}
	fmt.Printf("  verified sha256 %s\n", res.SHA256[:12])
	return 0
}

// unzipInto extracts an archive into destDir, rejecting entries that would escape
// it (the archive comes off the network).
func unzipInto(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = r.Close() }()

	if err = os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		clean := filepath.Clean(filepath.FromSlash(f.Name))
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("archive entry escapes target: %q", f.Name)
		}
		// Archives are built with a top-level folder; keep only its contents so the
		// runtime lands where locate.go expects it.
		parts := strings.SplitN(clean, string(filepath.Separator), 2)
		if len(parts) < 2 {
			continue
		}
		target := filepath.Join(destDir, parts[1])
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, openErr := f.Open()
		if openErr != nil {
			return openErr
		}
		out, createErr := os.Create(target)
		if createErr != nil {
			_ = rc.Close()
			return createErr
		}
		_, copyErr := io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func writeVerified(layout config.Layout, v verifiedFile, dir string) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "VERIFIED.json"), append(raw, '\n'), 0o644)
}
