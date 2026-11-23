package store

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Job kinds.
const (
	JobDispatchEvent  = "event.dispatch"
	JobDeliverWebhook = "webhook.deliver"
	JobRunAutomation  = "automation.run"
	JobSendEmail      = "email.send"
)

// Job is a unit of background work claimed by a worker.
type Job struct {
	ID          int64
	Kind        string
	Payload     json.RawMessage
	Attempts    int // including the current one
	MaxAttempts int
}

// Event provenance carried in the context, so appendEvent can tag events
// written by automation without changing every call site.
type sourceKey struct{}

type eventSource struct {
	source string
	ruleID uuid.UUID
}

// WithAutomation marks every event written with ctx as caused by rule ruleID.
// Automation rules never fire on such events, which prevents loops.
func WithAutomation(ctx context.Context, ruleID uuid.UUID) context.Context {
	return context.WithValue(ctx, sourceKey{}, eventSource{source: "automation", ruleID: ruleID})
}

// enqueueJob inserts a job. A non-empty dedupeKey makes the insert idempotent.
func enqueueJob(ctx context.Context, q querier, kind string, payload any, maxAttempts int, dedupeKey string) error {
	var key any
	if dedupeKey != "" {
		key = dedupeKey
	}
	_, err := q.Exec(ctx, `INSERT INTO jobs (kind, payload, max_attempts, dedupe_key) VALUES ($1, $2, $3, $4)
		ON CONFLICT (dedupe_key) DO NOTHING`, kind, payload, maxAttempts, key)
	return err
}

// ClaimJobs leases up to limit runnable jobs. SKIP LOCKED lets any number of
// workers and replicas poll concurrently without double-claiming. A worker
// that dies leaves its jobs to be re-claimed once the lease expires.
func (s *Store) ClaimJobs(ctx context.Context, limit int, lease time.Duration) ([]Job, error) {
	rows, err := s.pool.Query(ctx, `UPDATE jobs SET attempts = attempts + 1, locked_until = now() + $2::interval
		WHERE id IN (
			SELECT id FROM jobs
			WHERE state = 'pending' AND run_at <= now() AND (locked_until IS NULL OR locked_until < now())
			ORDER BY run_at, id LIMIT $1
			FOR UPDATE SKIP LOCKED)
		RETURNING id, kind, payload, attempts, max_attempts`, limit, lease.String())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Job, error) {
		var j Job
		return j, r.Scan(&j.ID, &j.Kind, &j.Payload, &j.Attempts, &j.MaxAttempts)
	})
}

// CompleteJob marks a job done.
func (s *Store) CompleteJob(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET state = 'done', finished_at = now(), locked_until = NULL, last_error = NULL
		WHERE id = $1`, id)
	return err
}

// FailJob records a failure and schedules a retry after backoff, or marks the
// job failed when it is permanent or out of attempts. It reports whether the
// job is now finally failed.
func (s *Store) FailJob(ctx context.Context, j Job, cause error, permanent bool, backoff time.Duration) (bool, error) {
	msg := cause.Error()
	if utf8.RuneCountInString(msg) > 1000 {
		msg = string([]rune(msg)[:1000])
	}
	final := permanent || j.Attempts >= j.MaxAttempts
	var err error
	if final {
		_, err = s.pool.Exec(ctx, `UPDATE jobs SET state = 'failed', finished_at = now(), locked_until = NULL, last_error = $2
			WHERE id = $1`, j.ID, msg)
	} else {
		_, err = s.pool.Exec(ctx, `UPDATE jobs SET run_at = now() + $2::interval, locked_until = NULL, last_error = $3
			WHERE id = $1`, j.ID, backoff.String(), msg)
	}
	return final, err
}

// PurgeFinished deletes finished jobs and old webhook delivery logs in bounded batches.
func (s *Store) PurgeFinished(ctx context.Context, jobsOlderThan, deliveriesOlderThan time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM jobs WHERE id IN (
		SELECT id FROM jobs WHERE state <> 'pending' AND finished_at < now() - $1::interval LIMIT 10000)`, jobsOlderThan.String())
	if err != nil {
		return 0, err
	}
	n := tag.RowsAffected()
	tag, err = s.pool.Exec(ctx, `DELETE FROM webhook_deliveries WHERE id IN (
		SELECT id FROM webhook_deliveries WHERE delivered_at < now() - $1::interval LIMIT 10000)`, deliveriesOlderThan.String())
	if err != nil {
		return n, err
	}
	return n + tag.RowsAffected(), nil
}

// PendingJobs returns the queue depth (for metrics and tests).
func (s *Store) PendingJobs(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE state = 'pending'`).Scan(&n)
	return n, err
}

// JobQueueStats is a point-in-time summary of the job queue for metrics.
type JobQueueStats struct {
	Due            int64   // pending, runnable now and not leased by a worker
	Running        int64   // pending and leased by a worker right now
	Scheduled      int64   // pending, run_at in the future (retry backoff, delayed)
	FailedLastHour int64   // failed permanently within the last hour
	OldestDueAge   float64 // seconds the oldest due job has been runnable; 0 if none
}

// JobQueueStats summarises the queue in one statement. The pending part reads
// the jobs_runnable_idx partial index, the failed part jobs_finished_idx.
func (s *Store) JobQueueStats(ctx context.Context) (JobQueueStats, error) {
	var st JobQueueStats
	err := s.pool.QueryRow(ctx, `
		WITH p AS (
			SELECT run_at, locked_until IS NOT NULL AND locked_until >= now() AS leased
			FROM jobs WHERE state = 'pending')
		SELECT
			count(*) FILTER (WHERE NOT leased AND run_at <= now()),
			count(*) FILTER (WHERE leased),
			count(*) FILTER (WHERE NOT leased AND run_at > now()),
			(SELECT count(*) FROM jobs WHERE state = 'failed' AND finished_at > now() - interval '1 hour'),
			coalesce(extract(epoch FROM now() - min(run_at) FILTER (WHERE NOT leased AND run_at <= now())), 0)::float8
		FROM p`).Scan(&st.Due, &st.Running, &st.Scheduled, &st.FailedLastHour, &st.OldestDueAge)
	return st, err
}
