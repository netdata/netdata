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
	for {
		serial := a.serial.Load()
		weight := int64(1)
		if serial {
			weight = a.capacity
		}
		if err := a.slots.Acquire(ctx, weight); err != nil {
			return 0, err
		}
		// A ServiceRoot response may change the policy while this request waits.
		if a.serial.Load() == serial {
			return weight, nil
		}
		a.slots.Release(weight)
	}
}

func (a *requestAdmission) release(weight int64) { a.slots.Release(weight) }
