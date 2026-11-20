package runctl

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/downloader"
	"github.com/hhx465453939/stemscribe/internal/registry"
)

// Installed describes a finished install, so a caller can print or record it.
type Installed struct {
	ID        string
	Kind      string
	Version   string
	Backend   string
	SHA256    string
	SourceURL string
	DestDir   string
}

// Progress is one download tick. Percent 0 means "size unknown", which callers must
// treat as no percentage rather than as 0% done.
type Progress struct {
	Percent int
	DoneMB  int64
	TotalMB int64
	// SHA256 is set on a final tick once verification succeeded. The CLI prints the
	// digest at that point, which keeps its output order identical to before the move.
	SHA256 string
	// Stage names non-download ticks: "start" announces what is about to happen,
	// "verify" carries the digest. Empty means a plain download tick.
	Stage     string
	Kind      string
	ID        string
	Version   string
	Backend   string
	SourceURL string
}

type verifiedFile struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Version     string    `json:"version,omitempty"`
	Backend     string    `json:"backend,omitempty"`
	SHA256      string    `json:"sha256"`
	SourceURL   string    `json:"source_url"`
	InstalledAt time.Time `json:"installed_at"`
}

// RequireChecksum refuses unverified bytes (ADR-025). A null+tbd entry means nobody
// has measured it yet, which is different from "measured and bad".
func RequireChecksum(id string, sha *string) error {
	if sha == nil || strings.TrimSpace(*sha) == "" {
		return fmt.Errorf("%s is not published yet (no measured sha256); refusing to install unverified bytes", id)
	}
	return nil
}

// FindRuntimeByID locates a runtime entry by product id.
func FindRuntimeByID(reg registry.Registry, id string) (registry.Runtime, bool) {
	for _, rt := range reg.Runtimes {
		if rt.ID == id {
			return rt, true
		}
	}
	return registry.Runtime{}, false
}

// runtimeDirName keeps a meaningless "-none" out of the user's work directory.
func runtimeDirName(version string, backend registry.Backend) string {
	if backend == "" || backend == registry.BackendNone {
		return version
	}
	return version + "-" + string(backend)
}

// Install downloads and verifies one runtime or model by id.
func Install(ctx context.Context, workdir, id string, onProgress func(Progress)) (Installed, error) {
	layout, err := config.NewLayout(workdir)
	if err != nil {
		return Installed{}, err
	}
	if err = layout.Ensure(); err != nil {
		return Installed{}, err
	}
	reg, err := registry.Builtin()
	if err != nil {
		return Installed{}, fmt.Errorf("load registry: %w", err)
	}
	if rt, ok := FindRuntimeByID(reg, id); ok {
		return installRuntime(ctx, layout, reg, rt, onProgress)
	}
	m, ok := reg.FindModel(id)
	if !ok {
		return Installed{}, fmt.Errorf("unknown model or runtime %q", id)
	}
	return installModel(ctx, layout, reg, m, onProgress)
}

func installRuntime(ctx context.Context, layout config.Layout, reg registry.Registry, rt registry.Runtime, onProgress func(Progress)) (Installed, error) {
	if err := RequireChecksum(rt.ID, rt.Archive.SHA256); err != nil {
		return Installed{}, err
	}
	mirror, ok := reg.MirrorByID("official")
	if !ok {
		return Installed{}, fmt.Errorf("registry has no official mirror")
	}
	url := reg.ResolveURL(mirror, rt.Archive.URL)
	tmpZip := filepath.Join(layout.Tmp(), rt.Archive.Dest)

	if onProgress != nil {
		onProgress(Progress{Stage: "start", Kind: "runtime", ID: rt.ID, Version: rt.Version,
			Backend: string(rt.Backend), SourceURL: url})
	}

	if err := fetchArtifact(ctx, url, tmpZip, rt.Archive.Size, *rt.Archive.SHA256, onProgress); err != nil {
		return Installed{}, err
	}
	destDir := filepath.Join(layout.Runtimes(), string(rt.Engine), runtimeDirName(rt.Version, rt.Backend))
	if err := unzipInto(tmpZip, destDir); err != nil {
		return Installed{}, fmt.Errorf("extract: %w", err)
	}
	_ = os.Remove(tmpZip)

	entry := filepath.Join(destDir, filepath.FromSlash(rt.Entrypoint))
	if _, err := os.Stat(entry); err != nil {
		return Installed{}, fmt.Errorf("archive did not contain %s: %w", rt.Entrypoint, err)
	}
	if err := writeVerified(layout, verifiedFile{
		ID: rt.ID, Kind: "runtime", Version: rt.Version, Backend: string(rt.Backend),
		SHA256: *rt.Archive.SHA256, SourceURL: url, InstalledAt: time.Now().UTC(),
	}, destDir); err != nil {
		return Installed{}, fmt.Errorf("record: %w", err)
	}
	return Installed{
		ID: rt.ID, Kind: "runtime", Version: rt.Version, Backend: string(rt.Backend),
		SHA256: *rt.Archive.SHA256, SourceURL: url, DestDir: destDir,
	}, nil
}

func installModel(ctx context.Context, layout config.Layout, reg registry.Registry, m registry.Model, onProgress func(Progress)) (Installed, error) {
	for _, f := range m.Files {
		if err := RequireChecksum(m.ID, f.SHA256); err != nil {
			return Installed{}, err
		}
	}
	mirror, ok := reg.MirrorByID("official")
	if !ok {
		return Installed{}, fmt.Errorf("registry has no official mirror")
	}
	destDir := filepath.Join(layout.Models(), m.Family, m.ID)
	for _, f := range m.Files {
		url := reg.ResolveURL(mirror, f.URL)
		dest := filepath.Join(destDir, filepath.FromSlash(f.Dest))
		if onProgress != nil {
			onProgress(Progress{Stage: "start", Kind: "model", ID: m.ID, SourceURL: url})
		}
		if err := fetchArtifact(ctx, url, dest, f.Size, *f.SHA256, onProgress); err != nil {
			return Installed{}, err
		}
	}
	src := reg.ResolveURL(mirror, m.Files[0].URL)
	if err := writeVerified(layout, verifiedFile{
		ID: m.ID, Kind: "model", SHA256: *m.Files[0].SHA256,
		SourceURL: src, InstalledAt: time.Now().UTC(),
	}, destDir); err != nil {
		return Installed{}, fmt.Errorf("record: %w", err)
	}
	return Installed{ID: m.ID, Kind: "model", SHA256: *m.Files[0].SHA256, SourceURL: src, DestDir: destDir}, nil
}

func fetchArtifact(ctx context.Context, url, dest string, size int64, sha string, onProgress func(Progress)) error {
	client := &downloader.Client{}
	lastPct := -5
	_, err := client.Fetch(ctx, downloader.Artifact{
		URL: url, DestPath: dest, Size: size, SHA256: sha,
	}, func(done, total, _ int64) {
		if onProgress == nil {
			return
		}
		p := Progress{DoneMB: done >> 20, TotalMB: total >> 20}
		if total > 0 {
			p.Percent = int(done * 100 / total)
			// 5% steps: the downloader fires every 512 KB, thousands of ticks on a 1.5 GB
			// model, which is unusable in a log or a progress bar.
			if p.Percent-lastPct < 5 && p.Percent != 100 {
				return
			}
			lastPct = p.Percent
		}
		onProgress(p)
	})
	if err != nil {
		return err
	}
	if onProgress != nil {
		onProgress(Progress{Percent: 100, SHA256: sha})
	}
	return nil
}

// unzipInto extracts an archive, rejecting entries that escape destDir.
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
		// Archives carry a top-level folder; keep only its contents so the runtime
		// lands where locate.go expects it.
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

// InstallState is the lifecycle of one install.
type InstallState string

const (
	InstallIdle    InstallState = "idle"
	InstallRunning InstallState = "running"
	InstallDone    InstallState = "done"
	InstallFailed  InstallState = "failed"
)

// InstallJob is what the model centre polls.
type InstallJob struct {
	ID      string       `json:"id"`
	Target  string       `json:"target"`
	State   InstallState `json:"state"`
	Percent int          `json:"percent"`
	Message string       `json:"message"`
}

// Installer runs at most one install at a time.
//
// One at a time is deliberate: two concurrent downloads of the same artifact would
// fight over the same .part file and the failure mode is a corrupted runtime.
type Installer struct {
	mu  sync.Mutex
	seq int
	job InstallJob
}

// NewInstaller creates an idle installer.
func NewInstaller() *Installer { return &Installer{job: InstallJob{State: InstallIdle}} }

// ErrInstallBusy is returned when an install is already running.
var ErrInstallBusy = fmt.Errorf("an install is already running")

// Start launches an install and returns its job id immediately.
func (i *Installer) Start(workdir, id string) (string, error) {
	i.mu.Lock()
	if i.job.State == InstallRunning {
		i.mu.Unlock()
		return "", ErrInstallBusy
	}
	i.seq++
	jobID := fmt.Sprintf("i-%d", i.seq)
	i.job = InstallJob{ID: jobID, Target: id, State: InstallRunning}
	i.mu.Unlock()

	go func() {
		_, err := Install(context.Background(), workdir, id, func(p Progress) {
			i.mu.Lock()
			if p.Percent > 0 {
				i.job.Percent = p.Percent
			}
			i.mu.Unlock()
		})
		i.mu.Lock()
		defer i.mu.Unlock()
		if err != nil {
			i.job.State = InstallFailed
			i.job.Message = err.Error()
			return
		}
		i.job.State = InstallDone
		i.job.Percent = 100
	}()
	return jobID, nil
}

// Status returns a snapshot of the current job.
func (i *Installer) Status() InstallJob {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.job
}
