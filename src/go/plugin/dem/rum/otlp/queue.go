// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"context"
	"time"
)

// runQueue batches queued items until ctx is done or the queue closes, then
// drains what remains once with the owner's shared final context. The owner
// joins producers before cancellation, so the drain sees every accepted item.
//
// export reports whether it attempted the batch. An attempted batch is consumed
// even on failure, since retrying could duplicate data; an unattempted normal
// batch moves to the final drain. Items the final drain cannot attempt go to
// drop. export must not retain the batch slice, which is reused.
func runQueue[T any](
	ctx context.Context,
	final func() context.Context,
	queue <-chan T,
	maxBatch int,
	export func(context.Context, []T) bool,
	drop func([]T),
) {
	ticker := time.NewTicker(batchPeriod)
	defer ticker.Stop()
	batch := make([]T, 0, maxBatch)
	flush := func(parent context.Context) bool {
		if len(batch) == 0 {
			return true
		}
		if !export(parent, batch) {
			return false
		}
		batch = batch[:0]
		return true
	}
normal:
	for {
		select {
		case <-ctx.Done():
			break normal
		case it, ok := <-queue:
			if !ok {
				break normal
			}
			batch = append(batch, it)
			if len(batch) >= maxBatch && !flush(ctx) {
				break normal
			}
		case <-ticker.C:
			if !flush(ctx) {
				break normal
			}
		}
	}
	// Use the owner's budget once, including for a batch whose normal export
	// never began.
	parent := final()
	flushFinal := func() {
		if !flush(parent) {
			drop(batch)
			batch = batch[:0]
		}
	}
	for {
		if len(batch) >= maxBatch {
			flushFinal()
		}
		select {
		case it, ok := <-queue:
			if !ok {
				flushFinal()
				return
			}
			batch = append(batch, it)
		default:
			flushFinal()
			return
		}
	}
}
