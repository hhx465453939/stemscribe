package runctl

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// QueueState is the lifecycle of one queued input.
type QueueState string

const (
	QueueWaiting  QueueState = "waiting"
	QueueRunning  QueueState = "running"
	QueueDone     QueueState = "done"
	QueueFailed   QueueState = "failed"
	QueueCanceled QueueState = "canceled"
)

// Runner is the slice of Service the queue needs. Keeping it an interface means the
// queue's ordering and failure semantics can be tested without a GPU, a model or a
// real video -- which is the only way those rules stay honest.
type Runner interface {
	Start(Options) (string, error)
	Get(id string) (Status, bool)
	Cancel(id string) error
}

// QueueItem is one queued file.
type QueueItem struct {
	ID          string     `json:"id"`
	Input       string     `json:"input"`
	RunID       string     `json:"run_id"`
	State       QueueState `json:"state"`
	Percent     int        `json:"percent"`
	Segments    int        `json:"segments"`
	ArtifactDir string     `json:"artifact_dir"`
	Message     string     `json:"message"`
	// Stage and ElapsedMS mirror the live run so the row can say "separating ·
	// 12:03" instead of only a percentage.
	Stage     string `json:"stage,omitempty"`
	ElapsedMS int    `json:"elapsed_ms"`
	// Stopping is true after a stop was requested and before the process tree is
	// gone; the button uses it to disable itself instead of firing twice.
	Stopping bool `json:"stopping,omitempty"`
	// Warnings carries machine codes the UI must surface on this row (for example
	// MODEL_SUBSTITUTED when the requested model was not installed). Without this
	// the substitution only existed in a log the row never showed.
	Warnings []string `json:"warnings,omitempty"`

	opts Options
}

// QueueStatus is the whole queue as the UI needs it.
type QueueStatus struct {
	Items   []QueueItem `json:"items"`
	Current string      `json:"current"`
	Done    int         `json:"done"`
	Total   int         `json:"total"`
}

// Queue runs inputs one after another.
//
// Serial is deliberate (ADR-027): two concurrent inference runs on one GPU fight for
// the same VRAM, and the queue's job is smooth handoff, not parallelism. Because the
// queue lives here rather than in the page, a reload mid-run loses nothing.
type Queue struct {
	mu      sync.Mutex
	runner  Runner
	items   []*QueueItem
	seq     int
	working bool
	poll    time.Duration
}

// NewQueue creates an empty queue backed by the given runner.
func NewQueue(r Runner) *Queue {
	return &Queue{runner: r, poll: 150 * time.Millisecond}
}

// mediaExts is what the intake accepts. Rejecting a .txt at the door is friendlier
// than queueing it and failing three stages later.
var mediaExts = map[string]bool{
	".mp4": true, ".mkv": true, ".mov": true, ".webm": true, ".avi": true,
	".flv": true, ".m4v": true, ".ts": true,
	".wav": true, ".mp3": true, ".m4a": true, ".flac": true, ".aac": true, ".ogg": true,
}

// Enqueue appends inputs and starts the worker if it is idle. It returns how many
// were accepted; rejected inputs are reported in the accompanying error slice.
func (q *Queue) Enqueue(inputs []string, base Options) ([]QueueItem, []string) {
	q.mu.Lock()
	added := make([]QueueItem, 0, len(inputs))
	rejected := make([]string, 0)
	for _, raw := range inputs {
		in := strings.TrimSpace(raw)
		if in == "" {
			continue
		}
		if !mediaExts[strings.ToLower(filepath.Ext(in))] {
			rejected = append(rejected, in)
			continue
		}
		q.seq++
		opts := base
		opts.Input = in
		it := &QueueItem{ID: fmt.Sprintf("q-%d", q.seq), Input: in, State: QueueWaiting, opts: opts}
		q.items = append(q.items, it)
		added = append(added, *it)
	}
	start := !q.working && len(added) > 0
	if start {
		q.working = true
	}
	q.mu.Unlock()

	if start {
		go q.work()
	}
	return added, rejected
}

// work drains the queue, one input at a time.
func (q *Queue) work() {
	for {
		q.mu.Lock()
		var next *QueueItem
		for _, it := range q.items {
			if it.State == QueueWaiting {
				next = it
				break
			}
		}
		if next == nil {
			q.working = false
			q.mu.Unlock()
			return
		}
		next.State = QueueRunning
		opts := next.opts
		q.mu.Unlock()

		q.run(next, opts)
	}
}

// run executes one item and mirrors its real progress until it settles.
//
// A failure is recorded on the item and the loop continues: one bad file must not
// strand the rest of the batch (AC-03).
func (q *Queue) run(item *QueueItem, opts Options) {
	runID, err := q.startWithRetry(opts)
	if err != nil {
		q.mu.Lock()
		item.State = QueueFailed
		item.Message = err.Error()
		q.mu.Unlock()
		return
	}
	q.mu.Lock()
	item.RunID = runID
	stopping := item.Stopping
	q.mu.Unlock()
	if stopping {
		// A stop arrived between "running" and the run id being known; honour it now.
		_ = q.runner.Cancel(runID)
	}

	for {
		st, ok := q.runner.Get(runID)
		if !ok {
			q.mu.Lock()
			item.State = QueueFailed
			item.Message = "run disappeared"
			q.mu.Unlock()
			return
		}
		q.mu.Lock()
		item.Percent = st.Percent
		item.Segments = st.Segments
		item.ArtifactDir = st.ArtifactDir
		item.Stage = st.Stage
		item.ElapsedMS = st.ElapsedMS
		item.Warnings = warningCodes(st.Events)
		q.mu.Unlock()

		switch st.State {
		case StateDone:
			q.mu.Lock()
			item.State = QueueDone
			item.Percent = 100
			item.Stage = ""
			q.mu.Unlock()
			return
		case StateCanceled:
			q.mu.Lock()
			item.State = QueueCanceled
			item.Stopping = false
			item.Stage = ""
			// The message is not surfaced for a cancel: "stopped by user" under a
			// "Stopped" label is noise. Keep whatever the run wrote for the record.
			q.mu.Unlock()
			return
		case StateFailed:
			q.mu.Lock()
			item.State = QueueFailed
			item.Message = st.Message
			item.Stage = ""
			q.mu.Unlock()
			return
		}
		time.Sleep(q.poll)
	}
}

// startWithRetry waits out a run that is already in flight. The queue is the normal
// driver, but someone can still POST /api/runs directly; treating that as a failed
// queue item would throw away work for no reason.
func (q *Queue) startWithRetry(opts Options) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 40; attempt++ {
		id, err := q.runner.Start(opts)
		if err == nil {
			return id, nil
		}
		lastErr = err
		time.Sleep(q.poll)
	}
	return "", lastErr
}

// warningCodes extracts the distinct machine codes from a run's warnings, in the
// order they first appeared.
func warningCodes(events []Event) []string {
	var out []string
	seen := map[string]bool{}
	for _, ev := range events {
		if ev.Kind != "stage.warn" || ev.Code == "" || seen[ev.Code] {
			continue
		}
		seen[ev.Code] = true
		out = append(out, ev.Code)
	}
	return out
}

// Snapshot returns a copy of the queue.
func (q *Queue) Snapshot() QueueStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := QueueStatus{Items: make([]QueueItem, 0, len(q.items)), Total: len(q.items)}
	for _, it := range q.items {
		out.Items = append(out.Items, *it)
		if it.State == QueueDone {
			out.Done++
		}
		if it.State == QueueRunning {
			out.Current = it.ID
		}
	}
	return out
}

// ErrQueueRunning refuses to drop an item that is already executing.
var ErrQueueRunning = fmt.Errorf("item is running; cannot remove")

// ErrQueueNotRunning is returned when a stop targets an item that is not running.
var ErrQueueNotRunning = fmt.Errorf("item is not running")

// ErrQueueNotFound is returned for an unknown id.
var ErrQueueNotFound = fmt.Errorf("queue item not found")

// Stop cancels the running item. The item stays in the list with state canceled
// once the run settles, and the worker continues with the next waiting item.
func (q *Queue) Stop(id string) error {
	q.mu.Lock()
	for _, it := range q.items {
		if it.ID != id {
			continue
		}
		if it.State != QueueRunning {
			q.mu.Unlock()
			return ErrQueueNotRunning
		}
		if it.Stopping {
			q.mu.Unlock()
			return nil // idempotent: a second click must not stack cancels
		}
		it.Stopping = true
		runID := it.RunID
		q.mu.Unlock()
		if runID == "" {
			// run() will see Stopping and cancel as soon as Start returns the id.
			return nil
		}
		return q.runner.Cancel(runID)
	}
	q.mu.Unlock()
	return ErrQueueNotFound
}

// Remove drops a waiting item. Running items are refused rather than killed: an
// interrupted transcription leaves a half-written bundle, which is worse than making
// the user wait for the current file to finish.
func (q *Queue) Remove(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, it := range q.items {
		if it.ID != id {
			continue
		}
		switch it.State {
		case QueueRunning:
			return ErrQueueRunning
		case QueueDone, QueueFailed:
			return fmt.Errorf("item already finished")
		}
		q.items = append(q.items[:i], q.items[i+1:]...)
		return nil
	}
	return ErrQueueNotFound
}
