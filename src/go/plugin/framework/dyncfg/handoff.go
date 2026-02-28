// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"time"
)

const DefaultDownstreamHandoffCap = time.Second

type BoundedSendResult uint8

const (
	BoundedSendOK BoundedSendResult = iota + 1
	BoundedSendContextDone
	BoundedSendTimeout
)

// BoundedSend sends value to ch using bounded wait:
// wait = min(remaining request deadline, maxWait), with maxWait used when no deadline exists.
func BoundedSend[T any](ctx context.Context, ch chan<- T, value T, maxWait time.Duration) BoundedSendResult {
	if maxWait <= 0 {
		maxWait = DefaultDownstreamHandoffCap
	}
	if ctx == nil {
		ctx = context.Background()
	}

	wait := maxWait
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if ctx.Err() != nil {
				return BoundedSendContextDone
			}
			return BoundedSendTimeout
		}
		if remaining < wait {
			wait = remaining
		}
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return BoundedSendContextDone
	case ch <- value:
		return BoundedSendOK
	case <-timer.C:
		return BoundedSendTimeout
	}
}
