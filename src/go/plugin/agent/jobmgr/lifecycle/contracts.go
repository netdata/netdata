// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"strings"
	"time"
)

// MaximumUIDBytes is the maximum encoded Function transaction UID.
const MaximumUIDBytes = 128

// ValidateUID checks whether uid is safe for line-oriented Function framing.
func ValidateUID(uid string) error {
	if uid == "" || len(uid) > MaximumUIDBytes ||
		strings.ContainsAny(uid, " \t\r\n\x00") {
		return errors.New("jobmgr lifecycle: invalid UID")
	}
	return nil
}

// Source identifies the ingress and lifecycle origin of an operation.
type Source uint8

const (
	SourceJobManager Source = iota + 1
	SourceFunction
)

// Valid reports whether source is a known scheduling source.
func (s Source) Valid() bool {
	return s == SourceJobManager || s == SourceFunction
}

// TaskClass identifies an independent TaskSupervisor pending queue.
type TaskClass uint8

const (
	TaskClassFrameworkControl TaskClass = iota + 1
	TaskClassGenericFunction
)

// Valid reports whether class is a known task scheduling class.
func (tc TaskClass) Valid() bool {
	return tc == TaskClassFrameworkControl ||
		tc == TaskClassGenericFunction
}

// OperationID uniquely identifies an operation within one run generation.
type OperationID uint64

// ResourceIdentity identifies one acknowledged resource generation.
type ResourceIdentity struct {
	ID         string
	Generation uint64
}

// Valid reports whether identity names a concrete resource generation.
func (ri ResourceIdentity) Valid() bool {
	return ri.ID != "" && ri.Generation != 0
}

// PreparedResource owns an unpublished resource until it is accepted or
// disposed.
type PreparedResource interface {
	Identity() ResourceIdentity
	AcceptStart(context.Context, uint64) (ReadyResource, error)
	Dispose(context.Context) error
}

// ReadyResource owns an accepted but not necessarily published resource.
type ReadyResource interface {
	Identity() ResourceIdentity
	Publish() error
	AbortReady(context.Context) error
	Stop(context.Context) error
	Finalize() error
}

// ControlStatus is the closed set of statuses the orchestration loop may emit
// without invoking a domain handler.
type ControlStatus uint16

const (
	ControlBadRequest      ControlStatus = 400
	ControlNotFound        ControlStatus = 404
	ControlPayloadTooLarge ControlStatus = 413
	ControlCancelled       ControlStatus = 499
	ControlInternal        ControlStatus = 500
	ControlUnavailable     ControlStatus = 503
	ControlDeadline        ControlStatus = 504
)

// Valid reports whether status belongs to the closed control-status set.
func (cs ControlStatus) Valid() bool {
	switch cs {
	case ControlBadRequest, ControlNotFound, ControlPayloadTooLarge,
		ControlCancelled, ControlInternal, ControlUnavailable, ControlDeadline:
		return true
	default:
		return false
	}
}

// ControlFramePlan is the bounded value accepted by the serialized frame
// writer for a control response.
type ControlFramePlan struct {
	UID    string
	Status ControlStatus
	Expiry int64
}

// Validate checks that a control frame can be encoded without consulting
// domain state.
func (cfp ControlFramePlan) Validate() error {
	if ValidateUID(cfp.UID) != nil {
		return errors.New("jobmgr lifecycle: invalid control UID")
	}
	if !cfp.Status.Valid() {
		return errors.New("jobmgr lifecycle: invalid control status")
	}
	if cfp.Expiry <= 0 {
		return errors.New("jobmgr lifecycle: invalid control expiry")
	}
	return nil
}

// Clock supplies orchestration time and cancellable one-shot timers.
type Clock interface {
	Now() time.Time
	Arm(kind string, delay time.Duration) (<-chan time.Time, func())
}

// ReusableTimer is a reusable timer owned by one orchestration loop.
type ReusableTimer interface {
	Arm(delay time.Duration) <-chan time.Time
	Stop()
}

// ReusableTimerClock optionally supplies reusable timers to avoid per-arm
// allocation on hot paths.
type ReusableTimerClock interface {
	NewTimer(kind string) ReusableTimer
}
