// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import "time"

// RuntimeGauge identifies a current value projected by its mutation owner.
type RuntimeGauge uint8

const (
	RuntimeGaugeOperationsActive RuntimeGauge = iota + 1
	RuntimeGaugeFunctionInvocationsActive
	RuntimeGaugeClaimKeysTracked
	RuntimeGaugeClaimWaiters
	RuntimeGaugeTasksActive
	RuntimeGaugeTasksQueued
	RuntimeGaugeJobsActive
)

// RuntimeCounter identifies a monotonic lifecycle observation.
type RuntimeCounter uint8

const (
	RuntimeCounterOperationsAdmitted RuntimeCounter = iota + 1
	RuntimeCounterOperationsRejected
	RuntimeCounterDuplicateUIDRejected
	RuntimeCounterShutdownRejected
	RuntimeCounterOperationTimeouts
	RuntimeCounterResultsDisposed
	RuntimeCounterTaskPanics
	RuntimeCounterFramesCommitted
	RuntimeCounterFrameFailures
	RuntimeCounterDirtyRuns
)

// RuntimeTimestamp identifies the start of the oldest current wait.
type RuntimeTimestamp uint8

const (
	RuntimeTimestampOldestOperation RuntimeTimestamp = iota + 1
	RuntimeTimestampOldestClaimWait
	RuntimeTimestampOldestTaskWait
)

// RuntimeObserver is a passive metrics projection. Implementations must not
// call back into lifecycle owners or influence their transitions.
type RuntimeObserver interface {
	SetRuntimeGauge(RuntimeGauge, int)
	AddRuntimeGauge(RuntimeGauge, int)
	AddRuntimeCounter(RuntimeCounter, uint64)
	SetRuntimeTimestamp(RuntimeTimestamp, time.Time)
}
