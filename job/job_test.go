package job

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type dummyJob struct {
	runFn func(ctx context.Context, progress func(Progress)) (Result, error)
}

func (d dummyJob) Run(ctx context.Context, progress func(Progress)) (Result, error) {
	return d.runFn(ctx, progress)
}

func TestQueueExecution(t *testing.T) {
	var mu sync.Mutex
	notified := 0
	q := NewQueue(2, func() {
		mu.Lock()
		notified++
		mu.Unlock()
	})

	id := q.Enqueue("test", dummyJob{
		runFn: func(ctx context.Context, progress func(Progress)) (Result, error) {
			progress(Progress{Done: 50, Total: 100, Unit: "%", Message: "halfway"})
			return Result{Outputs: []string{"/path/out"}, Summary: "done"}, nil
		},
	})

	// Wait for completion
	var rec JobRecord
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		r, ok := q.GetRecord(id)
		if ok && (r.Status == StatusSucceeded || r.Status == StatusFailed) {
			rec = r
			break
		}
	}

	if rec.Status != StatusSucceeded {
		t.Fatalf("expected job Succeeded, got status %v (err: %s)", rec.Status, rec.Error)
	}
	if len(rec.Result.Outputs) != 1 || rec.Result.Outputs[0] != "/path/out" {
		t.Errorf("unexpected outputs: %+v", rec.Result.Outputs)
	}
	if rec.Progress.Done != 50 {
		t.Errorf("expected Progress.Done == 50, got %v", rec.Progress.Done)
	}
}

func TestQueueSubscribe(t *testing.T) {
	q := NewQueue(1, nil)
	var mu sync.Mutex
	notified := 0
	unsub := q.Subscribe(func() {
		mu.Lock()
		notified++
		mu.Unlock()
	})
	defer unsub()

	q.Enqueue("test", dummyJob{
		runFn: func(ctx context.Context, progress func(Progress)) (Result, error) {
			progress(Progress{Done: 100, Total: 100})
			return Result{Summary: "done"}, nil
		},
	})

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	count := notified
	mu.Unlock()
	if count == 0 {
		t.Errorf("expected subscriber to be notified, got 0")
	}

	unsub()
	before := count
	q.Enqueue("test2", dummyJob{
		runFn: func(ctx context.Context, progress func(Progress)) (Result, error) {
			return Result{Summary: "done"}, nil
		},
	})
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	after := notified
	mu.Unlock()
	if after != before {
		t.Errorf("expected no notifications after unsub, got before=%d after=%d", before, after)
	}
}

func TestQueueCancellation(t *testing.T) {
	q := NewQueue(1, nil)
	started := make(chan struct{})

	id := q.Enqueue("test", dummyJob{
		runFn: func(ctx context.Context, progress func(Progress)) (Result, error) {
			close(started)
			<-ctx.Done()
			return Result{}, ctx.Err()
		},
	})

	<-started
	canceled := q.Cancel(id)
	if !canceled {
		t.Fatalf("expected Cancel to succeed")
	}

	// Wait for status update
	var rec JobRecord
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		r, ok := q.GetRecord(id)
		if ok && r.Status == StatusCanceled {
			rec = r
			break
		}
	}

	if rec.Status != StatusCanceled {
		t.Fatalf("expected job StatusCanceled, got %v", rec.Status)
	}
}

func TestQueueError(t *testing.T) {
	q := NewQueue(1, nil)
	expectedErr := errors.New("simulated error")

	id := q.Enqueue("test", dummyJob{
		runFn: func(ctx context.Context, progress func(Progress)) (Result, error) {
			return Result{}, expectedErr
		},
	})

	var rec JobRecord
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		r, ok := q.GetRecord(id)
		if ok && r.Status == StatusFailed {
			rec = r
			break
		}
	}

	if rec.Status != StatusFailed {
		t.Fatalf("expected job StatusFailed, got %v", rec.Status)
	}
	if rec.Error != expectedErr.Error() {
		t.Errorf("expected error %q, got %q", expectedErr.Error(), rec.Error)
	}
}
