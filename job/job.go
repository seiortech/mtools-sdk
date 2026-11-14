package job

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Common job errors.
var (
	ErrCanceled = errors.New("job canceled")
	ErrNotFound = errors.New("job not found")
)

// JobID identifies a specific execution unit.
type JobID string

// JobStatus represents current lifecycle state of a job.
type JobStatus int

const (
	StatusQueued JobStatus = iota
	StatusRunning
	StatusSucceeded
	StatusFailed
	StatusCanceled
)

// Progress holds real-time execution metrics.
type Progress struct {
	Done    float64 `json:"done"`
	Total   float64 `json:"total"`
	Unit    string  `json:"unit"`
	Message string  `json:"message"`
	LogLine string  `json:"log_line,omitempty"`
}

// Result describes the successful outcome of a job.
type Result struct {
	Outputs []string `json:"outputs"`
	Summary string   `json:"summary"`
}

// Job is one unit of background work.
type Job interface {
	Run(ctx context.Context, progress func(Progress)) (Result, error)
}

// Func adapts a standalone function to the Job interface.
type Func func(ctx context.Context, progress func(Progress)) (Result, error)

// Run invokes the underlying function.
func (f Func) Run(ctx context.Context, progress func(Progress)) (Result, error) {
	return f(ctx, progress)
}

// JobRecord represents the state and metrics of an enqueued or finished job.
type JobRecord struct {
	ID        JobID      `json:"id"`
	Tool      string     `json:"tool"`
	Status    JobStatus  `json:"status"`
	Progress  Progress   `json:"progress"`
	Logs      []string   `json:"logs,omitempty"`
	Result    Result     `json:"result"`
	Error     string     `json:"error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// Queue coordinates bounded background jobs with progress and cancellation.
type Queue struct {
	mu          sync.RWMutex
	concurrency int
	activeCount int
	records     map[JobID]*jobItem
	history     []JobID
	maxHistory  int
	idGen       atomic.Uint64
	onUpdate    func()
	subID       uint64
	subscribers map[uint64]func()
}

type jobItem struct {
	record JobRecord
	job    Job
	cancel context.CancelFunc
}

// NewQueue creates a job queue with bounded concurrency.
func NewQueue(concurrency int, onUpdate func()) *Queue {
	if concurrency <= 0 {
		concurrency = 2
	}
	return &Queue{
		concurrency: concurrency,
		records:     make(map[JobID]*jobItem),
		history:     make([]JobID, 0, 100),
		maxHistory:  100,
		onUpdate:    onUpdate,
		subscribers: make(map[uint64]func()),
	}
}

// Enqueue adds a job to the queue and starts processing if workers are available.
func (q *Queue) Enqueue(tool string, j Job) JobID {
	q.mu.Lock()
	defer q.mu.Unlock()

	id := JobID(fmt.Sprintf("job_%d_%d", time.Now().UnixMilli(), q.idGen.Add(1)))
	item := &jobItem{
		job: j,
		record: JobRecord{
			ID:        id,
			Tool:      tool,
			Status:    StatusQueued,
			CreatedAt: time.Now(),
		},
	}

	q.records[id] = item
	q.history = append(q.history, id)
	if len(q.history) > q.maxHistory {
		old := q.history[0]
		q.history = q.history[1:]
		delete(q.records, old)
	}

	q.scheduleLocked()
	q.notifyLocked()

	return id
}

// Cancel terminates a running or queued job.
func (q *Queue) Cancel(id JobID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, ok := q.records[id]
	if !ok {
		return false
	}

	if item.record.Status == StatusQueued {
		now := time.Now()
		item.record.Status = StatusCanceled
		item.record.EndedAt = &now
		q.notifyLocked()
		return true
	}

	if item.record.Status == StatusRunning && item.cancel != nil {
		item.record.Progress.Message = "Canceling..."
		item.cancel()
		q.notifyLocked()
		return true
	}

	return false
}

// GetRecord returns a snapshot of the job record.
func (q *Queue) GetRecord(id JobID) (JobRecord, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	item, ok := q.records[id]
	if !ok {
		return JobRecord{}, false
	}
	return item.record, true
}

func (q *Queue) scheduleLocked() {
	if q.activeCount >= q.concurrency {
		return
	}

	for _, id := range q.history {
		item := q.records[id]
		if item == nil || item.record.Status != StatusQueued {
			continue
		}

		q.activeCount++
		ctx, cancel := context.WithCancel(context.Background())
		item.cancel = cancel
		item.record.Status = StatusRunning

		go q.runWorker(item, ctx)

		if q.activeCount >= q.concurrency {
			break
		}
	}
}

func (q *Queue) runWorker(item *jobItem, ctx context.Context) {
	progressCallback := func(p Progress) {
		q.mu.Lock()
		item.record.Progress = p
		if p.LogLine != "" {
			item.record.Logs = append(item.record.Logs, p.LogLine)
			if len(item.record.Logs) > 200 {
				item.record.Logs = item.record.Logs[len(item.record.Logs)-200:]
			}
		}
		q.notifyLocked()
		q.mu.Unlock()
	}

	res, err := item.job.Run(ctx, progressCallback)

	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	item.record.EndedAt = &now
	q.activeCount--

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, ErrCanceled) {
			item.record.Status = StatusCanceled
		} else {
			item.record.Status = StatusFailed
			item.record.Error = err.Error()
		}
	} else {
		item.record.Status = StatusSucceeded
		item.record.Result = res
	}

	q.scheduleLocked()
	q.notifyLocked()
}

func (q *Queue) notifyLocked() {
	if q.onUpdate != nil {
		q.onUpdate()
	}
	for _, fn := range q.subscribers {
		fn()
	}
}

// Subscribe registers a listener invoked whenever the queue state changes.
// It returns an unsubscribe function.
func (q *Queue) Subscribe(fn func()) func() {
	if q == nil || fn == nil {
		return func() {}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.subscribers == nil {
		q.subscribers = make(map[uint64]func())
	}
	q.subID++
	id := q.subID
	q.subscribers[id] = fn
	return func() {
		q.mu.Lock()
		delete(q.subscribers, id)
		q.mu.Unlock()
	}
}
