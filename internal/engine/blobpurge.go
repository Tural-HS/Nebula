package engine

import (
	"context"
	"time"
)

const blobPurgeBatch = 200

// purgeBlobs removes blobs whose attachment rows are gone (deleted, cascaded
// or never committed), up to a few batches per tick. Deleting a blob is
// idempotent, so two workers racing on the same key do no harm.
func (e *Engine) purgeBlobs(ctx context.Context) {
	if e.Blobs == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	removed := 0
	for range 5 {
		keys, err := e.Store.DueBlobPurges(ctx, blobPurgeBatch, e.BlobPurgeDelay)
		if err != nil {
			if ctx.Err() == nil {
				e.Logger.Error("attachment blob purge", "error", err)
			}
			break
		}
		for _, key := range keys {
			if err := e.Blobs.Delete(ctx, key); err != nil {
				e.Logger.Warn("attachment blob purge: delete failed, will retry", "backend", e.Blobs.Name(), "error", err)
				return
			}
			if err := e.Store.BlobPurged(ctx, key); err != nil {
				e.Logger.Error("attachment blob purge", "error", err)
				return
			}
			removed++
		}
		if len(keys) < blobPurgeBatch {
			break
		}
	}
	if removed > 0 {
		e.Logger.Info("attachment blobs removed", "count", removed)
	}
}
