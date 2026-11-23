// Package worker runs background jobs from the database queue.
//
// Delivery is at-least-once: a job may run again if a worker dies after doing
// the work but before marking it done. Handlers must therefore be idempotent
// or tolerate duplicates (webhook receivers get a stable event ID to dedupe on).
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Handler processes one job. Returning an error schedules a retry with
// exponential backoff; wrap it with Permanent to fail the job immediately.
type Handler func(ctx context.Context, job store.Job) error

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks err as not worth retrying (bad input, deleted target, 4xx).
func Permanent(err error) error { return permanentError{err} }

// IsPermanent reports whether err was wrapped with Permanent.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

// Queue is the subset of the store the worker needs.
type Queue interface {
	ClaimJobs(ctx context.Context, limit int, lease time.Duration) ([]store.Job, error)
	CompleteJob(ctx context.Context, id int64) error
	FailJob(ctx context.Context, j store.Job, cause error, permanent bool, backoff time.Duration) (bool, error)
	PurgeFinished(ctx context.Context, jobsOlderThan, deliveriesOlderThan time.Duration) (int64, error)
}

// Options tune the worker.
type Options struct {
	Concurrency  int           // parallel jobs
	PollInterval time.Duration // idle polling delay
	Lease        time.Duration // a job is re-claimable after this; handler timeout is a bit less
	// ShutdownGrace is how long in-flight jobs may finish after shutdown starts;
	// then they are cancelled (and retried later by another worker).
	ShutdownGrace time.Duration
	// Wake, when set, triggers an immediate poll (LISTEN/NOTIFY); polling at
	// PollInterval stays as the fallback.
	Wake <-chan struct{}
	// Tick, when set, runs every TickInterval (default one minute) on the
	// worker's loop: periodic work such as the due-date scheduler. It must be
	// safe to run on several replicas at once.
	Tick         func(ctx context.Context)
	TickInterval time.Duration
}

// Worker polls the queue and runs handlers.
type Worker struct {
	q        Queue
	handlers map[string]Handler
	logger   *slog.Logger
	metrics  *observability.Metrics
	opts     Options
}

// New creates a worker. Unknown job kinds fail permanently.
func New(q Queue, handlers map[string]Handler, logger *slog.Logger, metrics *observability.Metrics, opts Options) *Worker {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	if opts.Lease <= 0 {
		opts.Lease = 2 * time.Minute
	}
	if opts.ShutdownGrace <= 0 {
		opts.ShutdownGrace = 20 * time.Second
	}
	if opts.TickInterval <= 0 {
		opts.TickInterval = time.Minute
	}
	return &Worker{q: q, handlers: handlers, logger: logger, metrics: metrics, opts: opts}
}

// Backoff returns the delay before retry number attempt (1-based):
// 15s, 30s, 1m, 2m ... capped at 6h, with ±20% jitter against thundering herds.
func Backoff(attempt int) time.Duration {
	d := 15 * time.Second
	for i := 1; i < attempt && d < 6*time.Hour; i++ {
		d *= 2
	}
	d = min(d, 6*time.Hour)
	jitter := 0.8 + 0.4*rand.Float64() // #nosec G404 -- jitter, not security
	return time.Duration(float64(d) * jitter)
}

// Run processes jobs until ctx is cancelled, then waits for in-flight jobs.
func (w *Worker) Run(ctx context.Context) {
	sem := make(chan struct{}, w.opts.Concurrency)
	var wg sync.WaitGroup
	// Handlers get their own context so a shutdown signal lets them finish
	// (bounded by the lease) instead of abandoning half-sent requests.
	jobCtx, cancelJobs := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelJobs()

	housekeeping := time.NewTicker(time.Hour)
	defer housekeeping.Stop()
	var tickC <-chan time.Time // nil blocks forever: no periodic work configured
	if w.opts.Tick != nil {
		tick := time.NewTicker(w.opts.TickInterval)
		defer tick.Stop()
		tickC = tick.C
		w.opts.Tick(ctx) // once at start-up, so nothing waits a full interval
	}
	w.logger.Info("worker started", "concurrency", w.opts.Concurrency)

	for {
		free := w.opts.Concurrency - len(sem)
		var jobs []store.Job
		if free > 0 {
			var err error
			jobs, err = w.q.ClaimJobs(ctx, free, w.opts.Lease)
			if err != nil && ctx.Err() == nil {
				w.logger.Error("claim jobs", "error", err)
			}
		}
		for _, j := range jobs {
			sem <- struct{}{}
			wg.Go(func() {
				defer func() { <-sem }()
				w.process(jobCtx, j)
			})
		}

		delay := w.opts.PollInterval
		if len(jobs) == free && free > 0 {
			delay = 10 * time.Millisecond // queue likely has more; keep draining
		}
		select {
		case <-ctx.Done():
			w.logger.Info("worker stopping; waiting for in-flight jobs", "grace", w.opts.ShutdownGrace.String())
			done := make(chan struct{})
			go func() { wg.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(w.opts.ShutdownGrace):
				w.logger.Warn("grace period over; cancelling in-flight jobs (they will be retried)")
				cancelJobs()
				<-done
			}
			return
		case <-housekeeping.C:
			if n, err := w.q.PurgeFinished(ctx, 7*24*time.Hour, 30*24*time.Hour); err != nil {
				w.logger.Error("purge finished jobs", "error", err)
			} else if n > 0 {
				w.logger.Info("purged finished jobs and old deliveries", "rows", n)
			}
		case <-tickC:
			w.opts.Tick(ctx)
		case _, ok := <-w.opts.Wake: // a nil channel blocks forever: no wake-ups configured
			if !ok {
				w.opts.Wake = nil // listener stopped: fall back to polling, never spin
			}
		case <-time.After(delay):
		}
	}
}

func (w *Worker) process(ctx context.Context, j store.Job) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, w.opts.Lease-10*time.Second)
	defer cancel()

	err := w.safeRun(ctx, j)
	result := "ok"
	log := w.logger.With("job_id", j.ID, "kind", j.Kind, "attempt", j.Attempts)
	if err == nil {
		if cerr := w.q.CompleteJob(context.WithoutCancel(ctx), j.ID); cerr != nil {
			log.Error("complete job", "error", cerr)
		}
	} else {
		final, ferr := w.q.FailJob(context.WithoutCancel(ctx), j, err, IsPermanent(err), Backoff(j.Attempts))
		if ferr != nil {
			log.Error("record job failure", "error", ferr)
		}
		if final {
			result = "failed"
			log.Error("job failed permanently", "error", err)
		} else {
			result = "retry"
			log.Warn("job failed; will retry", "error", err)
		}
	}
	if w.metrics != nil {
		w.metrics.JobsTotal.WithLabelValues(j.Kind, result).Inc()
		w.metrics.JobDuration.WithLabelValues(j.Kind).Observe(time.Since(start).Seconds())
	}
}

func (w *Worker) safeRun(ctx context.Context, j store.Job) (err error) {
	h, ok := w.handlers[j.Kind]
	if !ok {
		return Permanent(fmt.Errorf("no handler for job kind %q", j.Kind))
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	return h(ctx, j)
}
