package engine

import (
	"context"
	"time"
)

// overdueBatch bounds one scan; a backlog is worked off over several ticks.
const (
	overdueBatch    = 500
	recurrenceBatch = 200 // rules per tick; each creates its due items
)

// Tick runs the time-based scans (ADR-0009). The worker calls it every minute
// on every replica; the store makes each event idempotent, so concurrent runs
// are safe. Errors are logged and retried on the next tick.
func (e *Engine) Tick(ctx context.Context) {
	e.purgeBlobs(ctx)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for range 10 { // up to 10 batches per tick, then let jobs run
		n, err := e.Store.EmitOverdue(ctx, overdueBatch)
		if err != nil {
			if ctx.Err() == nil {
				e.Logger.Error("overdue scan", "error", err)
			}
			return
		}
		if n > 0 {
			e.Logger.Info("items became overdue", "count", n)
		}
		if n < overdueBatch {
			break
		}
	}
	// Recurring items first: a new item due today should be in today's digest.
	if n, err := e.Store.EmitRecurrences(ctx, recurrenceBatch); err != nil {
		if ctx.Err() == nil {
			e.Logger.Error("recurring items", "error", err)
		}
	} else if n > 0 {
		e.Logger.Info("recurring items created", "count", n)
	}
	n, err := e.Store.EmitOverdueDigests(ctx)
	if err != nil {
		if ctx.Err() == nil {
			e.Logger.Error("overdue digests", "error", err)
		}
		return
	}
	if n > 0 {
		e.Logger.Info("overdue digests emitted", "count", n)
	}
}
