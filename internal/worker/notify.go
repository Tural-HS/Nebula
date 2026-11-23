package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// JobsChannel is notified by a trigger whenever jobs are committed.
const JobsChannel = "nebula_jobs"

// Listen holds one dedicated connection that LISTENs for new jobs and signals
// the returned channel (coalesced: one pending signal at most). It reconnects
// with backoff; polling remains the fallback, so a lost connection only adds
// latency, never loses work. The channel closes when ctx is done.
func Listen(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) <-chan struct{} {
	wake := make(chan struct{}, 1)
	signal := func() {
		select {
		case wake <- struct{}{}:
		default: // a wake-up is already pending
		}
	}
	go func() {
		defer close(wake)
		backoff := time.Second
		for ctx.Err() == nil {
			err := listenOnce(ctx, pool, signal)
			if ctx.Err() != nil {
				return
			}
			logger.Warn("job listener disconnected; polling continues", "error", err, "retry_in", backoff.String())
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 30*time.Second)
		}
	}()
	return wake
}

func listenOnce(ctx context.Context, pool *pgxpool.Pool, signal func()) error {
	c, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	// The LISTEN state must never leak back into the pool.
	conn := c.Hijack()
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()

	if _, err := conn.Exec(ctx, "LISTEN "+JobsChannel); err != nil {
		return err
	}
	signal() // catch up on anything committed while we were not listening
	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		signal()
	}
}
