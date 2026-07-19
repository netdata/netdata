// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// Ingress translates process Function commands into lifecycle-kernel commands.
type Ingress struct {
	kernel jobmgr.AdmissionCommandPort
	clock  lifecycle.Clock
	quit   func()
	once   sync.Once
}

type inputBodyBudget struct {
	admission     *lifecycle.AdmissionLedger
	kernel        jobmgr.AdmissionCommandPort
	runGeneration uint64
	grants        <-chan lifecycle.AdmissionGrant
}

func NewIngress(kernel jobmgr.AdmissionCommandPort, clock lifecycle.Clock, quit func()) (*Ingress, error) {
	if kernel == nil || clock == nil || quit == nil {
		return nil, errors.New("jobmgr Function ingress adapter: incomplete ports")
	}
	return &Ingress{kernel: kernel, clock: clock, quit: quit}, nil
}

func (ibb *inputBodyBudget) GrowInputBody(ctx context.Context, token uint64, nextCapacity int64) (uint64, error) {
	token, err := ibb.admission.RequestInputBodyGrowth(ibb.runGeneration, token, nextCapacity)
	if err != nil {
		return 0, err
	}
	ibb.kernel.NotifyControlReady()
	select {
	case grant := <-ibb.grants:
		if grant.Kind != lifecycle.ReservationInputBodyGrowth || grant.InputBodyToken != token {
			_, _ = ibb.admission.AbortInputBody(token)
			return 0, errors.New("jobmgr Function ingress adapter: mismatched input body grant")
		}
		return token, nil
	case <-ctx.Done():
		_, abortErr := ibb.admission.AbortInputBody(token)
		return 0, errors.Join(ctx.Err(), abortErr)
	case <-ibb.kernel.Done():
		_, abortErr := ibb.admission.AbortInputBody(token)
		return 0, errors.Join(jobmgr.ErrStopped, abortErr)
	}
}

func (ibb *inputBodyBudget) CommitInputBodyGrowth(token uint64, capacity int64) error {
	wake, err := ibb.admission.CommitInputBodyGrowth(token, capacity)
	if wake {
		ibb.kernel.NotifyControlReady()
	}
	return err
}

func (ibb *inputBodyBudget) ReleaseInputBody(token uint64) error {
	wake, err := ibb.admission.AbortInputBody(token)
	if wake {
		ibb.kernel.NotifyControlReady()
	}
	return err
}

func (i *Ingress) HandleCall(ctx context.Context, call functionwire.Call) error {
	now := i.clock.Now()
	deadline := now.Add(call.Timeout)
	if call.Timeout == 0 {
		deadline = now
	}
	return i.kernel.Submit(ctx, jobmgr.Request{
		UID: call.UID, Source: lifecycle.SourceFunction, Route: call.Method,
		Args: append([]string(nil), call.Args...), Payload: call.Payload, ContentType: call.ContentType,
		Permissions: call.Access, CallerSource: call.Source, Timeout: call.Timeout,
		HasPayload: call.HasPayload, InputBodyToken: call.InputBodyToken, PayloadCapacity: call.PayloadCapacity,
		Deadline: deadline,
	})
}

func (i *Ingress) HandleCancel(ctx context.Context, uid string) error {
	return i.kernel.Cancel(ctx, uid)
}

func (i *Ingress) HandleReject(ctx context.Context, uid string, status int) error {
	return i.kernel.Reject(ctx, uid, lifecycle.ControlStatus(status))
}

func (i *Ingress) HandleQuit(context.Context) error {
	i.once.Do(i.quit)
	return nil
}
