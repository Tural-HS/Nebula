package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

type fakeStats struct {
	calls atomic.Int64
	err   error
	st    store.JobQueueStats
}

func (f *fakeStats) JobQueueStats(context.Context) (store.JobQueueStats, error) {
	f.calls.Add(1)
	return f.st, f.err
}

var quietLog = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestQueueStatsServesScrapesFromCache(t *testing.T) {
	src := &fakeStats{st: store.JobQueueStats{Due: 3, Running: 1, Scheduled: 2, FailedLastHour: 4, OldestDueAge: 12.5}}
	qs := NewQueueStats(src, time.Hour, quietLog)
	reg := prometheus.NewRegistry()
	reg.MustRegister(qs)

	// Before the first query: no gauges (not a misleading zero), only the error counter.
	if n := testutil.CollectAndCount(qs); n != 1 {
		t.Fatalf("collected %d metrics before the first refresh", n)
	}
	qs.refresh(context.Background())
	for range 50 { // a scrape storm
		if _, err := reg.Gather(); err != nil {
			t.Fatal(err)
		}
	}
	if c := src.calls.Load(); c != 1 {
		t.Fatalf("scrapes reached the database %d times", c)
	}
	want := `
# HELP nebula_job_queue_pending_jobs Pending background jobs: due = runnable now, running = leased by a worker, scheduled = waiting for run_at (retry backoff).
# TYPE nebula_job_queue_pending_jobs gauge
nebula_job_queue_pending_jobs{state="due"} 3
nebula_job_queue_pending_jobs{state="running"} 1
nebula_job_queue_pending_jobs{state="scheduled"} 2
# HELP nebula_job_queue_recently_failed_jobs Background jobs that failed permanently within the last hour.
# TYPE nebula_job_queue_recently_failed_jobs gauge
nebula_job_queue_recently_failed_jobs 4
# HELP nebula_job_queue_oldest_due_age_seconds How long the oldest due (runnable, not running) job has been waiting; 0 when none is due.
# TYPE nebula_job_queue_oldest_due_age_seconds gauge
nebula_job_queue_oldest_due_age_seconds 12.5
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"nebula_job_queue_pending_jobs", "nebula_job_queue_recently_failed_jobs", "nebula_job_queue_oldest_due_age_seconds"); err != nil {
		t.Fatal(err)
	}

	// A failing query keeps the last snapshot and counts the error.
	src.err = errors.New("db down")
	qs.refresh(context.Background())
	if v := testutil.ToFloat64(qs.errors); v != 1 {
		t.Fatalf("errors = %v", v)
	}
	if n := testutil.CollectAndCount(qs, "nebula_job_queue_pending_jobs"); n != 3 {
		t.Fatalf("snapshot dropped after an error: %d", n)
	}
}

func TestQueueStatsRunStopsWithContext(t *testing.T) {
	src := &fakeStats{}
	qs := NewQueueStats(src, 10*time.Millisecond, quietLog)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { qs.Run(ctx); close(done) }()
	time.Sleep(55 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
	if c := src.calls.Load(); c < 2 || c > 10 {
		t.Fatalf("unexpected number of queries: %d (bounded by the interval)", c)
	}
}

func TestJobQueueStatsQuery(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO jobs (kind, payload, state, run_at, locked_until, finished_at) VALUES
		('k', '{}', 'pending', now() - interval '90 seconds', NULL, NULL),             -- due, oldest
		('k', '{}', 'pending', now() - interval '10 seconds', now() - interval '1 s', NULL), -- due, lease expired
		('k', '{}', 'pending', now() - interval '5 minutes', now() + interval '1 minute', NULL), -- running
		('k', '{}', 'pending', now() + interval '1 hour', NULL, NULL),                -- scheduled (backoff)
		('k', '{}', 'failed', now() - interval '1 hour', NULL, now() - interval '5 minutes'), -- failed recently
		('k', '{}', 'failed', now() - interval '3 hours', NULL, now() - interval '2 hours'),  -- failed long ago
		('k', '{}', 'done', now() - interval '1 minute', NULL, now())`)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.New(pool).JobQueueStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Due != 2 || st.Running != 1 || st.Scheduled != 1 || st.FailedLastHour != 1 {
		t.Fatalf("stats: %+v", st)
	}
	if st.OldestDueAge < 89 || st.OldestDueAge > 120 {
		t.Fatalf("oldest due age %v, want ~90s", st.OldestDueAge)
	}
}
