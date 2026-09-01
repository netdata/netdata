// SPDX-License-Identifier: GPL-3.0-or-later

package contract

import (
	"context"
	"time"
)

type Mode string

const (
	ModeLifecycle      Mode = "lifecycle"
	ModeCephMultisite  Mode = "ceph_multisite"
	ModeAWSReplication Mode = "aws_replication"
)

type Endpoint string

const (
	EndpointSource      Endpoint = "source"
	EndpointDestination Endpoint = "destination"
)

type Status string

const (
	StatusSuccess Status = "success"
	StatusWaiting Status = "waiting"
	StatusFailed  Status = "failed"
)

type Reason string

const (
	ReasonNone              Reason = "none"
	ReasonRequest           Reason = "request"
	ReasonPayloadMismatch   Reason = "payload_mismatch"
	ReasonVisibilityTimeout Reason = "visibility_timeout"
	ReasonDeleteTimeout     Reason = "delete_timeout"
	ReasonCleanup           Reason = "cleanup"
	ReasonOwnership         Reason = "ownership"
	ReasonInternal          Reason = "internal"
)

type Operation string

const (
	OperationSetup            Operation = "setup"
	OperationPut              Operation = "put"
	OperationRead             Operation = "read"
	OperationList             Operation = "list"
	OperationDelete           Operation = "delete"
	OperationWriteVisibility  Operation = "write_visibility"
	OperationDeleteVisibility Operation = "delete_visibility"
	OperationReconcile        Operation = "reconcile"
	OperationCleanup          Operation = "cleanup"
)

// OperationResult reports one logical operation that was actually performed.
type OperationResult struct {
	Name     Operation
	Endpoint Endpoint
	Status   Status
	Reason   Reason
	Duration time.Duration
	Err      error
}

type ObjectiveResult struct {
	Performed bool
	Status    Status
	Lag       time.Duration
}

type CleanupResult struct {
	Pending      int
	Backpressure bool
}

type ProbeResult struct {
	Status           Status
	Reason           Reason
	PayloadCompared  bool
	PayloadMismatch  bool
	WriteVisibility  ObjectiveResult
	DeleteVisibility ObjectiveResult
}

// Result is the provider-neutral snapshot consumed by the root metric renderer.
type Result struct {
	Mode         Mode
	Operations   []OperationResult
	Cleanup      CleanupResult
	Probe        *ProbeResult
	LastTerminal *ProbeResult
	Err          error
}

type Engine interface {
	Check(ctx context.Context) error
	Collect(ctx context.Context) Result
	Cleanup(ctx context.Context)
}
