package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/store"
)

type fakeQueue struct {
	mu        sync.Mutex
	pending   []store.Job
	completed []int64
	failed    map[int64]struct {
		permanent bool
		err       string
	}
}

func (q *fakeQueue) ClaimJobs(_ context.Context, limit int, _ time.Duration) ([]store.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := min(limit, len(q.pending))
	out := q.pending[:n]
	q.pending = q.pending[n:]
	for i := range out {
		out[i].Attempts++
	}
	return out, nil
}

func (q *fakeQueue) CompleteJob(_ context.Context, id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.completed = append(q.completed, id)
	return nil
}

func (q *fakeQueue) FailJob(_ context.Context, j store.Job, cause error, permanent bool, _ time.Duration) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failed[j.ID] = struct {
		permanent bool
		err       string
	}{permanent, cause.Error()}
	return permanent || j.Attempts >= j.MaxAttempts, nil
}

func (q *fakeQueue) PurgeFinished(context.Context, time.Duration, time.Duration) (int64, error) {
	return 0, nil
}

func TestWorkerOutcomes(t *testing.T) {
	q := &fakeQueue{failed: map[int64]struct {
		permanent bool
		err       string
	}{}}
	q.pending = []store.Job{
		{ID: 1, Kind: "ok", MaxAttempts: 3},
		{ID: 2, Kind: "flaky", MaxAttempts: 3},
		{ID: 3, Kind: "bad", MaxAttempts: 3},
		{ID: 4, Kind: "unknown", MaxAttempts: 3},
		{ID: 5, Kind: "panics", MaxAttempts: 3},
	}
	handlers := map[string]Handler{
		"ok":     func(context.Context, store.Job) error { return nil },
		"flaky":  func(context.Context, store.Job) error { return errors.New("timeout") },
		"bad":    func(context.Context, store.Job) error { return Permanent(errors.New("410 gone")) },
		"panics": func(context.Context, store.Job) error { panic("boom") },
	}
	w := New(q, handlers, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.NewMetrics(),
		Options{Concurrency: 2, PollInterval: 5 * time.Millisecond, Lease: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	if len(q.completed) != 1 || q.completed[0] != 1 {
		t.Errorf("completed = %v", q.completed)
	}
	want := map[int64]bool{2: false, 3: true, 4: true, 5: false}
	for id, permanent := range want {
		f, ok := q.failed[id]
		if !ok || f.permanent != permanent {
			t.Errorf("job %d: failed=%v permanent=%v, want permanent=%v (%s)", id, ok, f.permanent, permanent, f.err)
		}
	}
	if q.failed[5].err != "handler panic: boom" {
		t.Errorf("panic not converted: %q", q.failed[5].err)
	}
}

func TestShutdownWaitsForInFlightJobs(t *testing.T) {
	q := &fakeQueue{failed: map[int64]struct {
		permanent bool
		err       string
	}{}, pending: []store.Job{{ID: 1, Kind: "slow", MaxAttempts: 1}}}
	var finished atomic.Bool
	started := make(chan struct{})
	handlers := map[string]Handler{"slow": func(ctx context.Context, _ store.Job) error {
		close(started)
		select {
		case <-time.After(200 * time.Millisecond):
			finished.Store(true)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	w := New(q, handlers, slog.New(slog.NewTextHandler(io.Discard, nil)), nil,
		Options{Concurrency: 1, PollInterval: 5 * time.Millisecond, Lease: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	<-started
	cancel() // shutdown while the job is running
	<-done
	if !finished.Load() || len(q.completed) != 1 {
		t.Fatal("in-flight job was abandoned on shutdown")
	}
}

func TestTickRunsAtStartAndPeriodically(t *testing.T) {
	q := &fakeQueue{failed: map[int64]struct {
		permanent bool
		err       string
	}{}}
	var ticks atomic.Int32
	w := New(q, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, Options{
		PollInterval: time.Hour, TickInterval: 20 * time.Millisecond,
		Tick: func(context.Context) { ticks.Add(1) },
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for ticks.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if n := ticks.Load(); n < 3 {
		t.Fatalf("tick ran %d times, want at least 3 (start-up + periodic)", n)
	}
}

func TestBackoff(t *testing.T) {
	prev := time.Duration(0)
	for a := 1; a <= 20; a++ {
		d := Backoff(a)
		if d < 12*time.Second || d > 6*time.Hour*12/10 {
			t.Fatalf("attempt %d: %v out of range", a, d)
		}
		if a <= 5 && d < prev/2 {
			t.Fatalf("attempt %d: backoff not growing (%v after %v)", a, d, prev)
		}
		prev = d
	}
}
