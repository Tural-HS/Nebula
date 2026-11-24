package engine

import (
	"context"
	"time"
)

// deletionBudget bounds the purge work of one tick (ADR-0039): a large
// workspace is removed over several ticks, in batches that each commit.
const deletionBudget = 20 * time.Second

// purgeDeletions works off requested project and workspace deletions. Every
// step is idempotent, so replicas racing on the same deletion and a crash
// half-way only repeat harmless work.
func (e *Engine) purgeDeletions(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, deletionBudget+5*time.Second)
	defer cancel()
	done, err := e.Store.PurgeDeletions(ctx, time.Now().Add(deletionBudget))
	for _, d := range done {
		// The workspace's own audit log is gone with it: the server log and
		// workspace_deletions are the record (ADR-0039).
		e.Logger.Info("deletion completed", "kind", d.Kind, "workspace_id", d.WorkspaceID,
			"project_id", d.ProjectID, "name", d.Name)
	}
	if err != nil && ctx.Err() == nil {
		e.Logger.Error("deletion purge", "error", err)
	}
}
