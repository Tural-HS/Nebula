package engine

import (
	"context"
	"time"
)

// overdueBatch bounds one scan; a backlog is worked off over several ticks.
const (
	overdueBatch    = 500
	recurrenceBatch = 200 // rules per tick; each creates its due items
	formulaBatches  = 20  // formula recompute batches (of 500 items) per tick
)

// Tick runs the time-based scans (ADR-0009). The worker calls it every minute
// on every replica; the store makes each event idempotent, so concurrent runs
// are safe. Errors are logged and retried on the next tick.
func (e *Engine) Tick(ctx context.Context) {
	e.purgeBlobs(ctx)
	e.purgeDeletions(ctx)
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
	// Monthly closes (ADR-0051) after recurring items, before formulas, so a
	// refilled field is recomputed in the same tick.
	if n, err := e.Store.EmitCycles(ctx); err != nil {
		if ctx.Err() == nil {
			e.Logger.Error("monthly close", "error", err)
		}
	} else if n > 0 {
		e.Logger.Info("months closed", "count", n)
	}
	e.recomputeFormulas(ctx)
	// Approval deadlines (ADR-0034): one approval.overdue per request.
	if n, err := e.Store.EmitApprovalOverdue(ctx, overdueBatch); err != nil {
		if ctx.Err() == nil {
			e.Logger.Error("approval deadlines", "error", err)
		}
	} else if n > 0 {
		e.Logger.Info("approval requests became overdue", "count", n)
	}
	e.slaTick(ctx)
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

// recomputeFormulas refreshes stored formula values (ADR-0030): first the
// daily pass of formulas that use today(), then up to formulaBatches
// batches of pending passes, so one large workspace cannot starve the tick.
func (e *Engine) recomputeFormulas(ctx context.Context) {
	if n, err := e.Store.ScheduleVolatileFormulas(ctx); err != nil {
		if ctx.Err() == nil {
			e.Logger.Error("schedule formula recompute", "error", err)
		}
		return
	} else if n > 0 {
		e.Logger.Info("daily formula recompute scheduled", "workspaces", n)
	}
	for range formulaBatches {
		n, err := e.Store.RecomputeFormulas(ctx)
		if err != nil {
			if ctx.Err() == nil {
				e.Logger.Error("formula recompute", "error", err)
			}
			return
		}
		if n == 0 {
			return
		}
	}
}

// slaTick recomputes stale SLA dues, then emits the at-risk and breach
// reminders (ADR-0045).
func (e *Engine) slaTick(ctx context.Context) {
	for range 5 {
		n, err := e.Store.RecomputeSLA(ctx, overdueBatch)
		if err != nil {
			if ctx.Err() == nil {
				e.Logger.Error("sla recompute", "error", err)
			}
			return
		}
		if n < overdueBatch {
			break
		}
	}
	if n, err := e.Store.EmitSLA(ctx, overdueBatch); err != nil {
		if ctx.Err() == nil {
			e.Logger.Error("sla reminders", "error", err)
		}
	} else if n > 0 {
		e.Logger.Info("sla reminders emitted", "count", n)
	}
}
