package runctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/pipeline"
	"github.com/hhx465453939/stemscribe/internal/store"
)

// BundleSummary is one bundle as the task board needs it.
//
// Every field is read from files on disk (meta.json, transcript.json). Nothing is
// synthesised: a board that invents rows is worse than an empty one, because the
// user cannot tell a working system from a broken one.
type BundleSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Dir         string `json:"dir"`
	Segments    int    `json:"segments"`
	DurationMS  int    `json:"duration_ms"`
	StagesDone  int    `json:"stages_done"`
	StagesTotal int    `json:"stages_total"`
	Engine      string `json:"engine"`
	EngineVer   string `json:"engine_version"`
	UpdatedAt   string `json:"updated_at"`
}

// Library lists the bundles under workdir/libraries, newest first.
func Library(workdir string) ([]BundleSummary, error) {
	layout, err := config.NewLayout(workdir)
	if err != nil {
		return nil, err
	}
	root := layout.Libraries()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []BundleSummary{}, nil
		}
		return nil, err
	}

	out := make([]BundleSummary, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		raw, readErr := os.ReadFile(filepath.Join(dir, "meta.json"))
		if readErr != nil {
			// A directory without metadata cannot be described; skipping is honest,
			// inventing a row is not.
			continue
		}
		var meta store.Meta
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		sum := BundleSummary{
			ID:     meta.VideoID,
			Title:  meta.Title,
			Status: meta.Status,
			Dir:    dir,
		}
		if sum.ID == "" {
			sum.ID = e.Name()
		}
		if sum.Title == "" {
			sum.Title = sum.ID
		}
		if eng, ok := meta.Provenance.Engines["transcribe"]; ok {
			sum.Engine = eng.Name
			sum.EngineVer = eng.Version
		}
		for _, st := range meta.Stages {
			sum.StagesTotal++
			if st.State == "ok" {
				sum.StagesDone++
			}
		}
		if segs, sErr := pipeline.ReadTranscript(filepath.Join(dir, "text", "transcript.json")); sErr == nil {
			sum.Segments = len(segs)
			// Duration comes from the transcript itself, not from the container: the
			// board should describe the work that was actually produced.
			if n := len(segs); n > 0 {
				sum.DurationMS = segs[n-1].EndMS
			}
		}
		if info, iErr := e.Info(); iErr == nil {
			sum.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339)
		}
		out = append(out, sum)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}
