package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Tural-HS/Nebula/internal/store"
)

// QueueStatsSource is the store query behind the job-queue gauges.
type QueueStatsSource interface {
	JobQueueStats(ctx context.Context) (store.JobQueueStats, error)
}

// QueueStats exports job-queue gauges. It queries the database on its own
// fixed interval and serves scrapes from the cached result, so the number of
// scrapers (or a scrape storm) never reaches the database. Until the first
// successful query it exports no gauges, rather than a misleading zero;
// nebula_job_queue_stats_last_success_timestamp_seconds shows staleness.
type QueueStats struct {
	src      QueueStatsSource
	interval time.Duration
	logger   *slog.Logger

	mu   sync.Mutex
	snap *store.JobQueueStats
	at   time.Time

	pending, failed, oldest, lastSuccess *prometheus.Desc
	errors                               prometheus.Counter
}

// NewQueueStats returns a collector; register it, then start Run.
func NewQueueStats(src QueueStatsSource, interval time.Duration, logger *slog.Logger) *QueueStats {
	return &QueueStats{
		src: src, interval: interval, logger: logger,
		pending: prometheus.NewDesc("nebula_job_queue_pending_jobs",
			"Pending background jobs: due = runnable now, running = leased by a worker, scheduled = waiting for run_at (retry backoff).",
			[]string{"state"}, nil),
		failed: prometheus.NewDesc("nebula_job_queue_recently_failed_jobs",
			"Background jobs that failed permanently within the last hour.", nil, nil),
		oldest: prometheus.NewDesc("nebula_job_queue_oldest_due_age_seconds",
			"How long the oldest due (runnable, not running) job has been waiting; 0 when none is due.", nil, nil),
		lastSuccess: prometheus.NewDesc("nebula_job_queue_stats_last_success_timestamp_seconds",
			"Unix time of the last successful job-queue statistics query.", nil, nil),
		errors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "nebula_job_queue_stats_errors_total",
			Help: "Failed job-queue statistics queries.",
		}),
	}
}

// Describe implements prometheus.Collector.
func (q *QueueStats) Describe(ch chan<- *prometheus.Desc) {
	ch <- q.pending
	ch <- q.failed
	ch <- q.oldest
	ch <- q.lastSuccess
	q.errors.Describe(ch)
}

// Collect implements prometheus.Collector from the cached snapshot.
func (q *QueueStats) Collect(ch chan<- prometheus.Metric) {
	q.errors.Collect(ch)
	q.mu.Lock()
	snap, at := q.snap, q.at
	q.mu.Unlock()
	if snap == nil {
		return
	}
	ch <- prometheus.MustNewConstMetric(q.pending, prometheus.GaugeValue, float64(snap.Due), "due")
	ch <- prometheus.MustNewConstMetric(q.pending, prometheus.GaugeValue, float64(snap.Running), "running")
	ch <- prometheus.MustNewConstMetric(q.pending, prometheus.GaugeValue, float64(snap.Scheduled), "scheduled")
	ch <- prometheus.MustNewConstMetric(q.failed, prometheus.GaugeValue, float64(snap.FailedLastHour))
	ch <- prometheus.MustNewConstMetric(q.oldest, prometheus.GaugeValue, snap.OldestDueAge)
	ch <- prometheus.MustNewConstMetric(q.lastSuccess, prometheus.GaugeValue, float64(at.UnixNano())/1e9)
}

// Run refreshes the snapshot immediately and then every interval until ctx ends.
func (q *QueueStats) Run(ctx context.Context) {
	t := time.NewTicker(q.interval)
	defer t.Stop()
	for {
		q.refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (q *QueueStats) refresh(ctx context.Context) {
	// Bounded well below the interval: a slow database must not pile up queries.
	qctx, cancel := context.WithTimeout(ctx, min(5*time.Second, q.interval/2))
	defer cancel()
	st, err := q.src.JobQueueStats(qctx)
	if err != nil {
		if ctx.Err() == nil {
			q.errors.Inc()
			q.logger.Warn("job queue statistics", "error", err)
		}
		return // keep the last snapshot; its timestamp shows the age
	}
	q.mu.Lock()
	q.snap, q.at = &st, time.Now()
	q.mu.Unlock()
}
