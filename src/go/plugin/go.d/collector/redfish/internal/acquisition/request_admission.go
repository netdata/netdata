// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"sync/atomic"

	"golang.org/x/sync/semaphore"
)

// All SDK connections of a job share the configured HTTP concurrency budget.
// A service declaring MultipleHTTPRequests=false takes the entire budget for
// each request. Bootstrap is serial until the first ServiceRoot is read.
type requestAdmission struct {
	slots    *semaphore.Weighted
	capacity int64
	serial   atomic.Bool
}

func newRequestAdmission(limit int) *requestAdmission {
	a := &requestAdmission{
		slots:    semaphore.NewWeighted(int64(max(limit, 1))),
		capacity: int64(max(limit, 1)),
	}
	a.serial.Store(true)
	return a
}

func (a *requestAdmission) acquire(ctx context.Context) (int64, error) {
	weight := int64(1)
	if a.serial.Load() {
		weight = a.capacity
	}
	return weight, a.slots.Acquire(ctx, weight)
}

func (a *requestAdmission) release(weight int64) { a.slots.Release(weight) }
