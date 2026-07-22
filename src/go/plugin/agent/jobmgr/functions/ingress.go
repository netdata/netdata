// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// Ingress translates process Function commands into lifecycle-kernel commands.
type Ingress struct {
	kernel jobmgr.CommandPort // command port to submit calls to
	clock  lifecycle.Clock    // clock for deadline derivation
	quit   func()             // cancels the ingress reader
}

func NewIngress(kernel jobmgr.CommandPort, clock lifecycle.Clock, quit func()) (*Ingress, error) {
	if kernel == nil || clock == nil || quit == nil {
		return nil, errors.New("jobmgr Function ingress adapter: incomplete ports")
	}
	return &Ingress{kernel: kernel, clock: clock, quit: quit}, nil
}

func (i *Ingress) HandleCall(ctx context.Context, call functionwire.Call) error {
	var deadline time.Time
	if call.Timeout > 0 {
		deadline = i.clock.Now().Add(call.Timeout)
	}
	return i.kernel.Submit(ctx, jobmgr.Request{
		UID: call.UID, Source: lifecycle.SourceFunction, Route: call.Method,
		Args: slices.Clone(call.Args), Payload: call.Payload, ContentType: call.ContentType,
		Permissions: call.Access, CallerSource: call.Source, Timeout: call.Timeout,
		HasPayload: call.HasPayload,
		Deadline:   deadline,
	})
}

func (i *Ingress) HandleCancel(ctx context.Context, uid string) error {
	return i.kernel.Cancel(ctx, uid)
}

func (i *Ingress) HandleReject(ctx context.Context, uid string, status int) error {
	return i.kernel.Reject(ctx, uid, lifecycle.ControlStatus(status))
}

func (i *Ingress) HandleQuit(context.Context) error {
	i.quit()
	return nil
}
