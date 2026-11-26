package runctl

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeRunner is the smallest Runner that can be cancelled: it flips the run to
// canceled when Cancel is called and reports a stage/elapsed snapshot.
type fakeRunner struct {
	mu   sync.Mutex
	seq  int
	runs map[string]*Status
}

func newFakeRunner() *fakeRunner { return &fakeRunner{runs: map[string]*Status{}} }

func (f *fakeRunner) Start(Options) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	id := fmt.Sprintf("r-%d", f.seq)
	f.runs[id] = &Status{
		ID:         id,
		State:      StateRunning,
		Stage:      "separate",
		ElapsedMS:  1234,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		StageTotal: 5,
	}
	return id, nil
}

func (f *fakeRunner) Get(id string) (Status, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.runs[id]
	if !ok {
		return Status{}, false
	}
	return *st, true
}

func (f *fakeRunner) Cancel(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.runs[id]
	if !ok {
		return ErrRunNotFound
	}
	if st.State != StateRunning {
		return ErrNotRunning
	}
	st.State = StateCanceled
	st.Stage = ""
	return nil
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func findQueueItem(t *testing.T, q *Queue, id string) QueueItem {
	t.Helper()
	for _, it := range q.Snapshot().Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("queue item %s disappeared", id)
	return QueueItem{}
}

// Stopping the running item must mark it canceled and let the next waiting file
// start; one user cancel must not strand the rest of the batch.
func TestQueueStopRunningContinuesNext(t *testing.T) {
	f := newFakeRunner()
	q := NewQueue(f)
	added, rejected := q.Enqueue([]string{"a.mp4", "b.mp4"}, Options{})
	if len(added) != 2 || len(rejected) != 0 {
		t.Fatalf("added=%d rejected=%v", len(added), rejected)
	}

	waitFor(t, "first item running", func() bool {
		it := findQueueItem(t, q, added[0].ID)
		return it.State == QueueRunning && it.RunID != ""
	})

	if err := q.Stop(added[0].ID); err != nil {
		t.Fatalf("stop: %v", err)
	}

	waitFor(t, "first item canceled", func() bool {
		return findQueueItem(t, q, added[0].ID).State == QueueCanceled
	})
	waitFor(t, "second item running", func() bool {
		return findQueueItem(t, q, added[1].ID).State == QueueRunning
	})

	first := findQueueItem(t, q, added[0].ID)
	if first.Stopping {
		t.Fatal("stopping flag must clear once the item settled")
	}
}

func TestQueueSnapshotCarriesStageAndElapsed(t *testing.T) {
	f := newFakeRunner()
	q := NewQueue(f)
	added, _ := q.Enqueue([]string{"a.mp4"}, Options{})
	waitFor(t, "item running", func() bool {
		return findQueueItem(t, q, added[0].ID).Stage == "separate"
	})
	it := findQueueItem(t, q, added[0].ID)
	if it.ElapsedMS != 1234 {
		t.Fatalf("elapsed_ms = %d want 1234", it.ElapsedMS)
	}
}

func TestQueueStopRejectsWaitingAndUnknown(t *testing.T) {
	f := newFakeRunner()
	q := NewQueue(f)
	added, _ := q.Enqueue([]string{"a.mp4", "b.mp4"}, Options{})
	if err := q.Stop(added[1].ID); err != ErrQueueNotRunning {
		t.Fatalf("stop waiting item = %v want ErrQueueNotRunning", err)
	}
	if err := q.Stop("q-999"); err != ErrQueueNotFound {
		t.Fatalf("stop unknown = %v want ErrQueueNotFound", err)
	}
	waitFor(t, "first running before stop", func() bool {
		return findQueueItem(t, q, added[0].ID).State == QueueRunning
	})
	if err := q.Stop(added[0].ID); err != nil {
		t.Fatalf("stop running = %v", err)
	}
	waitFor(t, "first canceled", func() bool {
		return findQueueItem(t, q, added[0].ID).State == QueueCanceled
	})
}

func TestServiceCancelMarksCanceled(t *testing.T) {
	old := runFn
	defer func() { runFn = old }()

	started := make(chan struct{})
	runFn = func(ctx context.Context, _ Options) (Outcome, error) {
		close(started)
		<-ctx.Done()
		// The pipeline wraps the engine error with the stage name; keep that shape
		// so the test proves errors.Is survives the wrapper.
		return Outcome{}, fmt.Errorf("failed at separate: %w", ctx.Err())
	}

	svc := NewService()
	id, err := svc.Start(Options{Input: "x.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	// Let real time pass so the frozen elapsed value is distinguishable from 0;
	// without the terminal-time freeze a canceled run reported elapsed_ms = 0.
	time.Sleep(20 * time.Millisecond)
	if err := svc.Cancel(id); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitFor(t, "canceled state", func() bool {
		st, ok := svc.Get(id)
		return ok && st.State == StateCanceled
	})
	st, _ := svc.Get(id)
	if st.Message != "stopped by user" {
		t.Fatalf("message = %q", st.Message)
	}
	if st.ElapsedMS < 10 {
		t.Fatalf("elapsed_ms = %d; a canceled run must keep the time it ran", st.ElapsedMS)
	}
	if err := svc.Cancel(id); err != ErrNotRunning {
		t.Fatalf("second cancel = %v want ErrNotRunning", err)
	}
	if err := svc.Cancel("r-999"); err != ErrRunNotFound {
		t.Fatalf("unknown cancel = %v want ErrRunNotFound", err)
	}
}
