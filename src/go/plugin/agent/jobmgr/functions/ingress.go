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
		return nil, errors.New("Function ingress adapter: incomplete ports")
	}
	return &Ingress{kernel: kernel, clock: clock, quit: quit}, nil
}

func (budget *inputBodyBudget) GrowInputBody(ctx context.Context, token uint64, nextCapacity int64) (uint64, error) {
	token, err := budget.admission.RequestInputBodyGrowth(budget.runGeneration, token, nextCapacity)
	if err != nil {
		return 0, err
	}
	budget.kernel.NotifyControlReady()
	select {
	case grant := <-budget.grants:
		if grant.Kind != lifecycle.ReservationInputBodyGrowth || grant.InputBodyToken != token {
			_, _ = budget.admission.AbortInputBody(token)
			return 0, errors.New("Function ingress adapter: mismatched input body grant")
		}
		return token, nil
	case <-ctx.Done():
		_, abortErr := budget.admission.AbortInputBody(token)
		return 0, errors.Join(ctx.Err(), abortErr)
	case <-budget.kernel.Done():
		_, abortErr := budget.admission.AbortInputBody(token)
		return 0, errors.Join(jobmgr.ErrStopped, abortErr)
	}
}

func (budget *inputBodyBudget) CommitInputBodyGrowth(token uint64, capacity int64) error {
	wake, err := budget.admission.CommitInputBodyGrowth(token, capacity)
	if wake {
		budget.kernel.NotifyControlReady()
	}
	return err
}

func (budget *inputBodyBudget) ReleaseInputBody(token uint64) error {
	wake, err := budget.admission.AbortInputBody(token)
	if wake {
		budget.kernel.NotifyControlReady()
	}
	return err
}

func (ingress *Ingress) HandleCall(ctx context.Context, call functionwire.Call) error {
	now := ingress.clock.Now()
	deadline := now.Add(call.Timeout)
	if call.Timeout == 0 {
		deadline = now
	}
	return ingress.kernel.Submit(ctx, jobmgr.Request{
		UID: call.UID, Source: lifecycle.SourceFunction, Route: call.Method,
		Args: append([]string(nil), call.Args...), Payload: call.Payload, ContentType: call.ContentType,
		Permissions: call.Access, CallerSource: call.Source, Timeout: call.Timeout,
		HasPayload: call.HasPayload, InputBodyToken: call.InputBodyToken, PayloadCapacity: call.PayloadCapacity,
		Deadline: deadline,
	})
}

func (ingress *Ingress) HandleCancel(ctx context.Context, uid string) error {
	return ingress.kernel.Cancel(ctx, uid)
}

func (ingress *Ingress) HandleReject(ctx context.Context, uid string, status int) error {
	return ingress.kernel.Reject(ctx, uid, lifecycle.ControlStatus(status))
}

func (ingress *Ingress) HandleQuit(context.Context) error {
	ingress.once.Do(ingress.quit)
	return nil
}
