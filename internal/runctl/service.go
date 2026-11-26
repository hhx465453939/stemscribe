package runctl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hhx465453939/stemscribe/internal/pipeline"
)

// State is the lifecycle of one run.
type State string

const (
	StateQueued   State = "queued"
	StateRunning  State = "running"
	StateDone     State = "done"
	StateFailed   State = "failed"
	StateCanceled State = "canceled"
)

// Event is one step of the run, retained so a client that attaches late can still
// show what happened instead of a blank panel.
type Event struct {
	At    string `json:"at"`
	Stage string `json:"stage"`
	Kind  string `json:"kind"`
	// Code is the machine-readable tag (MODEL_SUBSTITUTED, ENGINE_OOM, ...). The UI
	// needs it to decide what to surface; the prose in Note is for humans.
	Code string `json:"code,omitempty"`
	Note string `json:"note,omitempty"`
}

// Status is the whole observable state of a run. It is a snapshot returned by
// value, so a reader can never observe a half-written struct.
type Status struct {
	ID          string `json:"id"`
	State       State  `json:"state"`
	Input       string `json:"input"`
	Model       string `json:"model"`
	Stage       string `json:"stage"`
	StageIndex  int    `json:"stage_index"`
	StageTotal  int    `json:"stage_total"`
	Percent     int    `json:"percent"`
	Segments    int    `json:"segments"`
	ArtifactDir string `json:"artifact_dir"`
	Message     string `json:"message"`
	// ElapsedMS is computed on read while the run is in flight, so the UI can show
	// a clock without the service inventing ticks (ADR-023).
	ElapsedMS int     `json:"elapsed_ms"`
	Events    []Event `json:"events"`
	StartedAt string  `json:"started_at"`
	EndedAt   string  `json:"ended_at"`
}

// Service owns the run state machine.
//
// One run at a time is deliberate: a second concurrent run on the same bundle
// would fight over the same files, and the failure mode (two writers, one
// checkpoint) is far worse than a "please wait" message.
type Service struct {
	mu      sync.Mutex
	seq     int
	byID    map[string]*Status
	latest  string
	cancels map[string]context.CancelFunc
}

// NewService creates an empty service.
func NewService() *Service {
	return &Service{byID: map[string]*Status{}, cancels: map[string]context.CancelFunc{}}
}

// runFn is the run implementation, injectable so cancel semantics can be tested
// without a GPU or a model.
var runFn = Run

// ErrBusy is returned when a run is already in progress.
var ErrBusy = fmt.Errorf("a run is already in progress")

// ErrRunNotFound is returned by Cancel for an unknown id.
var ErrRunNotFound = fmt.Errorf("run not found")

// ErrNotRunning is returned when a finished run is asked to stop.
var ErrNotRunning = fmt.Errorf("run is not running")

// Start launches a run and returns immediately with its id. Progress is read via
// Get or Latest; nothing here blocks the caller.
func (s *Service) Start(o Options) (string, error) {
	s.mu.Lock()
	if cur, ok := s.byID[s.latest]; ok && (cur.State == StateRunning || cur.State == StateQueued) {
		s.mu.Unlock()
		return "", ErrBusy
	}
	s.seq++
	id := fmt.Sprintf("r-%d", s.seq)
	total := TotalStages(separationEnabled(o.Engine))
	st := &Status{
		ID:         id,
		State:      StateQueued,
		Input:      o.Input,
		Model:      o.ModelID,
		StageTotal: total,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	s.byID[id] = st
	s.latest = id
	ctx, cancel := context.WithCancel(context.Background())
	s.cancels[id] = cancel
	s.mu.Unlock()

	go s.execute(ctx, id, o)
	return id, nil
}

// Cancel stops a running run. The child processes are killed through the context
// (engine.wireCancel), and the run settles as StateCanceled -- not failed, because
// "I pressed stop" is not a defect.
func (s *Service) Cancel(id string) error {
	s.mu.Lock()
	st, ok := s.byID[id]
	if !ok {
		s.mu.Unlock()
		return ErrRunNotFound
	}
	if st.State != StateRunning && st.State != StateQueued {
		s.mu.Unlock()
		return ErrNotRunning
	}
	cancel := s.cancels[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// disabledNote is the note pipeline.Run attaches to a stage that settings switched
// off, as opposed to one the checkpoint already proved valid. Both arrive as
// stage.skip, but only the second one means "this work is done" -- counting the
// first made the bar report 9/7.
const disabledNote = "disabled by settings"

func (s *Service) execute(ctx context.Context, id string, o Options) {
	s.mu.Lock()
	st := s.byID[id]
	st.State = StateRunning
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.cancels, id)
		s.mu.Unlock()
	}()

	done := 0
	userEmit := o.Emit
	o.Emit = func(ev pipeline.Event) {
		s.mu.Lock()
		st.Stage = ev.Stage
		switch ev.Kind {
		case "stage.end":
			done++
			st.StageIndex = done
			st.Percent = percent(done, st.StageTotal)
		case "stage.skip":
			// A skipped-because-valid stage is complete work; a skipped-because-disabled
			// one is not in the denominator at all. Treating them alike produced "9/7".
			if ev.Note != disabledNote {
				done++
				st.StageIndex = done
				st.Percent = percent(done, st.StageTotal)
			}
		case "stage.start":
			// Keep the bar honest: a stage is only credited when it ends. On start we
			// only advance the label, so the bar never runs ahead of the work.
			if ev.Stage != "" {
				st.Stage = ev.Stage
			}
		}
		if len(st.Events) < 200 {
			st.Events = append(st.Events, Event{
				At:    ev.TS,
				Stage: ev.Stage,
				Kind:  ev.Kind,
				Note:  ev.Note,
			})
		}
		s.mu.Unlock()
		if userEmit != nil {
			userEmit(ev)
		}
	}

	out, err := runFn(ctx, o)

	s.mu.Lock()
	defer s.mu.Unlock()
	st.EndedAt = time.Now().UTC().Format(time.RFC3339)
	// Freeze the clock at the terminal transition. snapshot() only recomputes it
	// while the run is live, so without this a canceled or finished row would show
	// 0s -- and "stopped after 40s" is exactly the fact the user needs to see.
	if started, perr := time.Parse(time.RFC3339, st.StartedAt); perr == nil {
		if ms := time.Since(started).Milliseconds(); ms > 0 {
			st.ElapsedMS = int(ms)
		}
	}
	if err != nil {
		// A canceled run is not a failure: it is the outcome the user asked for.
		// The pipeline wraps the engine error, so errors.Is is the only safe check.
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			st.State = StateCanceled
			st.Message = "stopped by user"
			return
		}
		st.State = StateFailed
		st.Message = err.Error()
		return
	}
	st.State = StateDone
	st.Segments = out.Segments
	st.ArtifactDir = out.BundleDir
	st.Percent = 100
	st.Stage = ""
}

func percent(done, total int) int {
	if total <= 0 {
		return 0
	}
	p := done * 100 / total
	if p > 100 {
		p = 100
	}
	return p
}

// Get returns a snapshot of one run.
func (s *Service) Get(id string) (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.byID[id]
	if !ok {
		return Status{}, false
	}
	return snapshot(st), true
}

// Latest returns the most recent run, which is what a freshly loaded page needs.
func (s *Service) Latest() (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.byID[s.latest]
	if !ok {
		return Status{}, false
	}
	return snapshot(st), true
}

// snapshot copies a status by value and fills the read-time clock. Callers hold mu.
func snapshot(st *Status) Status {
	out := *st
	if st.State == StateRunning || st.State == StateQueued {
		if started, err := time.Parse(time.RFC3339, st.StartedAt); err == nil {
			if ms := time.Since(started).Milliseconds(); ms > 0 {
				out.ElapsedMS = int(ms)
			}
		}
	}
	return out
}
