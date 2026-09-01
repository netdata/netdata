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
	ReasonAuthentication    Reason = "authentication"
	ReasonAuthorization     Reason = "authorization"
	ReasonBucket            Reason = "bucket"
	ReasonBucketVersioning  Reason = "bucket_versioning"
	ReasonReplicationPolicy Reason = "replication_policy"
	ReasonRequest           Reason = "request"
	ReasonPayloadMismatch   Reason = "payload_mismatch"
	ReasonVisibilityTimeout Reason = "visibility_timeout"
	ReasonDeleteTimeout     Reason = "delete_timeout"
	ReasonCleanup           Reason = "cleanup"
	ReasonOwnership         Reason = "ownership"
	ReasonBackpressure      Reason = "backpressure"
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
	Requests int64
}

type ObjectiveResult struct {
	Performed bool
	Status    Status
	Reason    Reason
	Lag       time.Duration
	Objective time.Duration
}

type CleanupResult struct {
	Pending      int
	Backpressure bool
	Attempted    int
	Removed      int
	Failed       int
}

type TerminalResult struct {
	Status           Status
	Reason           Reason
	PayloadMismatch  bool
	WriteVisibility  ObjectiveResult
	DeleteVisibility ObjectiveResult
}

// Result is the provider-neutral snapshot consumed by the root metric renderer.
type Result struct {
	Mode       Mode
	Operations []OperationResult
	Cleanup    CleanupResult
	Terminal   *TerminalResult
}

type Engine interface {
	Check(ctx context.Context) error
	Collect(ctx context.Context) Result
	Cleanup(ctx context.Context)
}
