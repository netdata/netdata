// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// journeyStopAt reserves reporting time inside the effective admitted budget.
// Sample the shared clock first so sampling latency cannot extend the deadline.
func journeyStopAt(deadline, admitted time.Time) (int64, error) {
	now, err := monotonicNow()
	if err != nil {
		return 0, err
	}
	reserve := min(5*time.Second, max(0, deadline.Sub(admitted))/5)
	return now.Milliseconds() + (time.Until(deadline) - reserve).Milliseconds(), nil
}

func deadlineReason(caller, attempt context.Context) synthetic.TerminationReason {
	callerDeadline, ok := caller.Deadline()
	attemptDeadline, _ := attempt.Deadline()
	if ok && !callerDeadline.After(attemptDeadline) {
		return synthetic.CallerDeadline
	}
	return synthetic.ExecutionDeadline
}
