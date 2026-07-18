// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const (
	// These match the existing plugins.d command-line and parameter ceilings.
	maximumRequestArgumentBytes = 15_487
	maximumRequestArguments     = 1_024
	maximumRequestMetadataBytes = maximumRequestArgumentBytes
)

// Request is one immutable command admission value. LaneKey is supplied only
// for Job Manager commands. Generic Function calls receive independent
// invocation lanes; the Function catalog selects a shared lane only for
// resource-scoped commands. The submitting adapter transfers ownership of Args
// and Payload until the request reaches terminal disposal.
type Request struct {
	UID             string
	LaneKey         string
	Source          lifecycle.Source
	Route           string
	Args            []string
	Payload         []byte
	ContentType     string
	Permissions     string
	CallerSource    string
	Timeout         time.Duration
	HasPayload      bool
	InputBodyToken  uint64
	PayloadCapacity int64
	Deadline        time.Time
}

// Validate checks the bounded admission invariants independent of mutable
// orchestration state.
func (request Request) Validate() error {
	if lifecycle.ValidateUID(request.UID) != nil ||
		request.Route == "" ||
		!request.Source.Valid() {
		return errors.New("jobmgr: invalid request")
	}
	if (request.Source == lifecycle.SourceJobManager && request.LaneKey == "") ||
		(request.Source == lifecycle.SourceFunction && request.LaneKey != "") {
		return errors.New("jobmgr: invalid request")
	}
	if len(request.LaneKey) > maximumRequestMetadataBytes ||
		len(request.Route) > maximumRequestMetadataBytes ||
		len(request.ContentType) > maximumRequestMetadataBytes ||
		len(request.Permissions) > maximumRequestMetadataBytes ||
		len(request.CallerSource) > maximumRequestMetadataBytes ||
		request.Timeout < 0 ||
		len(request.Args) > maximumRequestArguments {
		return errors.New("jobmgr: request metadata exceeds bounds")
	}
	argumentBytes := 0
	for _, argument := range request.Args {
		if len(argument) > maximumRequestArgumentBytes-argumentBytes {
			return errors.New("jobmgr: request arguments exceed bounds")
		}
		argumentBytes += len(argument)
	}
	if request.InputBodyToken == 0 {
		if request.PayloadCapacity != 0 || cap(request.Payload) != 0 {
			return errors.New("jobmgr: unreserved input payload")
		}
		return nil
	}
	if !request.HasPayload ||
		request.PayloadCapacity <= 0 ||
		int64(cap(request.Payload)) != request.PayloadCapacity ||
		int64(len(request.Payload)) > request.PayloadCapacity {
		return errors.New("jobmgr: invalid input payload reservation")
	}
	return nil
}

// CommandPort is the narrow command-ingress capability supplied to adapters.
// It intentionally exposes no kernel state or lifecycle construction.
type CommandPort interface {
	Submit(context.Context, Request) error
	Reject(context.Context, string, lifecycle.ControlStatus) error
	Cancel(context.Context, string) error
}

// AdmissionCommandPort adds the wake and terminal signals needed by a bounded
// input-body admission adapter.
type AdmissionCommandPort interface {
	CommandPort
	NotifyControlReady()
	Done() <-chan struct{}
}

// PreparedCommandPort accepts one already prepared Job Manager plan. It is
// used by typed in-process adapters whose immutable values cannot be
// represented by the wire-oriented Request fields without an object registry.
type PreparedCommandPort interface {
	SubmitPrepared(context.Context, Request, WorkPlan) error
	SubmitPreparedAndWait(context.Context, Request, WorkPlan) error
}
