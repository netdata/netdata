// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

type ProcessIngressState string

const (
	ProcessIngressPaused    ProcessIngressState = "paused"
	ProcessIngressLive      ProcessIngressState = "live"
	ProcessIngressContained ProcessIngressState = "contained"
)

type ProcessIngressCensus struct {
	State                  ProcessIngressState
	CapsulePayloadBytes    int
	ActiveDeliveries       int
	ReadReturns            int
	WaitingReadReturns     int
	DiscardedReads         int
	BudgetOperations       int
	ReaderStarts           int
	CapsulePayloadCapacity int
	RunGeneration          uint64
	CapsuleDiscardingLine  bool
	CapsulePayloadActive   bool
	CapabilityAttached     bool
	PendingBody            bool
	BodyBindingAttached    bool
	BodySuspended          bool
}

type processInputPort interface {
	functionwire.Consumer
	functionwire.BodyBudget
	Generation() uint64
	SuspendInputBody(nextGeneration, token uint64) error
	AdoptInputBody(token uint64) error
}

type ProcessBinding struct {
	port      processInputPort
	admission *lifecycle.AdmissionLedger
}

func NewProcessBinding(
	kernel jobmgr.AdmissionCommandPort,
	admission *lifecycle.AdmissionLedger,
	runGeneration uint64,
	grants <-chan lifecycle.AdmissionGrant,
	clock lifecycle.Clock,
	quit func(),
) (ProcessBinding, error) {
	ingress, err := NewIngress(kernel, clock, quit)
	if err != nil {
		return ProcessBinding{}, err
	}
	if admission == nil || runGeneration == 0 || grants == nil {
		return ProcessBinding{}, errors.New("jobmgr Function process ingress: incomplete binding")
	}
	return ProcessBinding{
		admission: admission,
		port: &boundProcessInput{
			Ingress: ingress,
			inputBodyBudget: inputBodyBudget{
				admission: admission, kernel: kernel, runGeneration: runGeneration, grants: grants,
			},
		},
	}, nil
}

type boundProcessInput struct {
	*Ingress
	inputBodyBudget
}

func (bpi *boundProcessInput) Generation() uint64 { return bpi.runGeneration }

func (bpi *boundProcessInput) SuspendInputBody(nextGeneration, token uint64) error {
	if err := bpi.admission.SuspendInputBody(bpi.runGeneration, nextGeneration, token); err != nil {
		return err
	}
	bpi.inputBodyBudget.kernel.NotifyControlReady()
	return nil
}

func (bpi *boundProcessInput) AdoptInputBody(token uint64) error {
	return bpi.admission.AdoptInputBody(bpi.runGeneration, token)
}

// ProcessIngress owns the one process-lifetime Function reader and swaps only
// its generation-scoped delivery capability during Agent restart.
type ProcessIngress struct {
	active        processInputPort
	body          processInputPort
	idle          *sync.Cond
	capsule       *functionwire.InputCapsule
	boundary      *capsuleBoundary
	admission     *lifecycle.AdmissionLedger
	state         ProcessIngressState
	runGeneration uint64
	deliveries    int
	budgetOps     int
	bodyToken     uint64
	pauseNext     uint64
	fencedToken   uint64
	readerStarts  int
	mu            sync.Mutex
	growth        bool
	pauseSealed   bool
	bodySuspended bool
}

func NewProcessIngress(reader io.Reader, admission *lifecycle.AdmissionLedger) (*ProcessIngress, error) {
	if reader == nil || admission == nil {
		return nil, errors.New("jobmgr Function process ingress: incomplete process authority")
	}
	ingress := &ProcessIngress{admission: admission, state: ProcessIngressPaused}
	ingress.idle = sync.NewCond(&ingress.mu)
	boundary, err := newCapsuleBoundary(ingress)
	if err != nil {
		return nil, err
	}
	capsule, err := functionwire.NewInputCapsule(reader, boundary)
	if err != nil {
		return nil, err
	}
	ingress.capsule = capsule
	ingress.boundary = boundary
	return ingress, nil
}

func (pi *ProcessIngress) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("jobmgr Function process ingress: nil reader context")
	}
	pi.mu.Lock()
	if pi.readerStarts != 0 || pi.state == ProcessIngressContained {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: reader already started or contained")
	}
	pi.readerStarts++
	pi.mu.Unlock()
	return pi.capsule.Run(ctx, pi.boundary)
}

func (pi *ProcessIngress) Adopt(ctx context.Context, binding ProcessBinding) error {
	if ctx == nil ||
		binding.port == nil ||
		binding.admission == nil ||
		binding.admission != pi.admission ||
		binding.port.Generation() == 0 {
		return errors.New("jobmgr Function process ingress: invalid binding")
	}
	pi.mu.Lock()
	if pi.state != ProcessIngressPaused ||
		pi.active != nil ||
		pi.deliveries != 0 ||
		pi.budgetOps != 0 ||
		pi.growth {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: adopt outside drained pause")
	}
	if pi.pauseNext != 0 && binding.port.Generation() != pi.pauseNext {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: adopted generation differs from pause target")
	}
	pi.mu.Unlock()
	if err := pi.boundary.prepareAdopt(ctx); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			pi.boundary.abortAdopt()
		}
	}()
	pi.mu.Lock()
	defer pi.mu.Unlock()
	if pi.state != ProcessIngressPaused ||
		pi.active != nil ||
		pi.deliveries != 0 ||
		pi.budgetOps != 0 ||
		pi.growth {
		return errors.New("jobmgr Function process ingress: adopt state changed during preparation")
	}
	pi.active = binding.port
	previousGeneration := pi.runGeneration
	pi.runGeneration = binding.port.Generation()
	if pi.body != nil || pi.bodySuspended {
		if pi.bodyToken == 0 {
			pi.active = nil
			pi.runGeneration = previousGeneration
			return errors.New("jobmgr Function process ingress: adopted body has no token")
		}
		if pi.body != nil || !pi.bodySuspended {
			pi.active = nil
			pi.runGeneration = previousGeneration
			return errors.New("jobmgr Function process ingress: adopted body retained its old run binding")
		}
		if err := binding.port.AdoptInputBody(pi.bodyToken); err != nil {
			pi.active = nil
			pi.runGeneration = previousGeneration
			return err
		}
		pi.body = binding.port
		pi.bodySuspended = false
	}
	pi.pauseNext = 0
	pi.state = ProcessIngressLive
	pi.boundary.commitAdopt()
	committed = true
	pi.idle.Broadcast()
	return nil
}

func (pi *ProcessIngress) SealPause() error {
	pi.mu.Lock()
	if pi.state != ProcessIngressLive || pi.active == nil {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: pause outside live state")
	}
	if pi.pauseSealed {
		pi.mu.Unlock()
		return nil
	}
	pi.mu.Unlock()
	if err := pi.boundary.sealPause(); err != nil {
		return err
	}
	pi.mu.Lock()
	if pi.state != ProcessIngressLive || pi.active == nil || pi.pauseSealed {
		pi.mu.Unlock()
		_ = pi.boundary.rollbackPause()
		return errors.New("jobmgr Function process ingress: state changed while sealing pause")
	}
	pi.pauseSealed = true
	pi.mu.Unlock()
	return nil
}

func (pi *ProcessIngress) DrainPause(ctx context.Context, nextGeneration uint64) error {
	if ctx == nil {
		return errors.New("jobmgr Function process ingress: nil pause context")
	}
	pi.mu.Lock()
	valid := pi.state == ProcessIngressLive && pi.active != nil && pi.pauseSealed
	pi.mu.Unlock()
	if !valid {
		return errors.New("jobmgr Function process ingress: drain outside sealed pause")
	}
	if err := pi.boundary.drainPause(ctx); err != nil {
		return err
	}
	pi.mu.Lock()
	if pi.state != ProcessIngressLive ||
		pi.active == nil ||
		pi.deliveries != 0 ||
		pi.budgetOps != 0 ||
		pi.growth {
		pi.state = ProcessIngressPaused
		pi.active = nil
		pi.pauseSealed = false
		pi.pauseNext = 0
		pi.idle.Broadcast()
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: pause did not drain process input")
	}
	var pauseErr error
	if pi.body != nil {
		if pi.bodyToken == 0 {
			pauseErr = errors.New("jobmgr Function process ingress: partial payload has no admission token")
		} else {
			pauseErr = pi.body.SuspendInputBody(nextGeneration, pi.bodyToken)
			if pauseErr == nil {
				pi.body = nil
				pi.bodySuspended = true
			}
		}
	} else if pi.bodySuspended {
		pauseErr = errors.New("jobmgr Function process ingress: body was already suspended")
	}
	pi.state = ProcessIngressPaused
	pi.active = nil
	pi.pauseSealed = false
	if pauseErr == nil {
		pi.pauseNext = nextGeneration
	} else {
		pi.pauseNext = 0
	}
	pi.idle.Broadcast()
	pi.mu.Unlock()
	return pauseErr
}

func (pi *ProcessIngress) Pause(ctx context.Context, nextGeneration uint64) error {
	if err := pi.SealPause(); err != nil {
		return err
	}
	if err := pi.DrainPause(ctx, nextGeneration); err != nil {
		pi.mu.Lock()
		rollback := pi.state == ProcessIngressLive && pi.pauseSealed
		if rollback {
			pi.pauseSealed = false
		}
		pi.mu.Unlock()
		if rollback {
			_ = pi.boundary.rollbackPause()
		}
		return err
	}
	return nil
}

func (pi *ProcessIngress) Fence(ctx context.Context) error {
	if ctx == nil {
		return errors.New("jobmgr Function process ingress: nil fence context")
	}
	pi.mu.Lock()
	if pi.state != ProcessIngressPaused ||
		pi.active != nil ||
		pi.deliveries != 0 ||
		pi.budgetOps != 0 ||
		pi.growth {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: final fence outside drained pause")
	}
	body := pi.body
	bodySuspended := pi.bodySuspended
	token := pi.bodyToken
	discardErr := pi.capsule.DiscardPausedPayload(token)
	pi.body = nil
	pi.bodySuspended = false
	pi.bodyToken = 0
	pi.fencedToken = token
	pi.idle.Broadcast()
	pi.mu.Unlock()

	var releaseErr error
	if body != nil {
		releaseErr = body.ReleaseInputBody(token)
	} else if bodySuspended {
		wake, err := pi.admission.AbortInputBody(token)
		releaseErr = err
		if err == nil && wake {
			releaseErr = errors.New("jobmgr Function process ingress: suspended body release exposed unrelated grantable work")
		}
	}
	fenceErr := pi.boundary.fence(ctx)
	boundary := pi.boundary.census()
	if !boundary.CapabilityAttached {
		pi.mu.Lock()
		pi.state = ProcessIngressContained
		pi.runGeneration = 0
		pi.idle.Broadcast()
		pi.mu.Unlock()
	}
	return errors.Join(discardErr, releaseErr, fenceErr)
}

func (pi *ProcessIngress) Census() ProcessIngressCensus {
	pi.mu.Lock()
	defer pi.mu.Unlock()
	return pi.censusLocked()
}

func (pi *ProcessIngress) censusLocked() ProcessIngressCensus {
	boundary := pi.boundary.census()
	var contained functionwire.ContainedInputCensus
	if pi.state == ProcessIngressContained {
		contained = pi.capsule.ContainedCensus()
	}
	return ProcessIngressCensus{
		State:                  pi.state,
		RunGeneration:          pi.runGeneration,
		ReaderStarts:           pi.readerStarts,
		ReadReturns:            boundary.ReadReturns,
		WaitingReadReturns:     boundary.WaitingReadReturns,
		DiscardedReads:         boundary.DiscardedReads,
		CapabilityAttached:     boundary.CapabilityAttached,
		CapsulePayloadActive:   contained.PayloadActive,
		CapsulePayloadBytes:    contained.PayloadBytes,
		CapsulePayloadCapacity: contained.PayloadCapacity,
		CapsuleDiscardingLine:  contained.DiscardingLine,
		ActiveDeliveries:       pi.deliveries,
		BudgetOperations:       pi.budgetOps,
		PendingBody:            pi.body != nil || pi.bodySuspended,
		BodyBindingAttached:    pi.body != nil,
		BodySuspended:          pi.bodySuspended,
	}
}

func (pi *ProcessIngress) HandleCall(ctx context.Context, call functionwire.Call) error {
	port, ok := pi.acquireDelivery()
	if !ok {
		return pi.discardCall(call)
	}
	defer pi.releaseDelivery()
	err := port.HandleCall(ctx, call)
	if call.InputBodyToken != 0 {
		pi.mu.Lock()
		if pi.body == port {
			pi.body = nil
			pi.bodyToken = 0
		}
		pi.mu.Unlock()
	}
	return pi.dispositionDeliveryError(port, err)
}

func (pi *ProcessIngress) HandleCancel(ctx context.Context, uid string) error {
	port, ok := pi.acquireDelivery()
	if !ok {
		return nil
	}
	defer pi.releaseDelivery()
	return pi.dispositionDeliveryError(port, port.HandleCancel(ctx, uid))
}

func (pi *ProcessIngress) HandleReject(ctx context.Context, uid string, status int) error {
	port, ok := pi.acquireDelivery()
	if !ok {
		return nil
	}
	defer pi.releaseDelivery()
	return pi.dispositionDeliveryError(port, port.HandleReject(ctx, uid, status))
}

func (pi *ProcessIngress) HandleQuit(ctx context.Context) error {
	port, ok := pi.acquireDelivery()
	if !ok {
		return nil
	}
	defer pi.releaseDelivery()
	return port.HandleQuit(ctx)
}

func (pi *ProcessIngress) GrowInputBody(ctx context.Context, token uint64, nextCapacity int64) (uint64, error) {
	pi.mu.Lock()
	for pi.state == ProcessIngressPaused {
		pi.idle.Wait()
	}
	if pi.state != ProcessIngressLive || pi.active == nil || pi.growth {
		pi.mu.Unlock()
		return 0, errors.New("jobmgr Function process ingress: payload growth outside live state")
	}
	port := pi.body
	if token == 0 {
		if pi.body != nil || pi.bodySuspended {
			pi.mu.Unlock()
			return 0, errors.New("jobmgr Function process ingress: payload started outside live state")
		}
		port = pi.active
		pi.body = port
	} else if port == nil || token != pi.bodyToken {
		pi.mu.Unlock()
		return 0, errors.New("jobmgr Function process ingress: stale payload growth")
	}
	pi.budgetOps++
	pi.mu.Unlock()
	result, err := port.GrowInputBody(ctx, token, nextCapacity)
	pi.mu.Lock()
	if err != nil {
		pi.budgetOps--
		if token == 0 && pi.body == port {
			pi.body = nil
		}
		pi.idle.Broadcast()
		pi.mu.Unlock()
		return 0, err
	}
	pi.bodyToken = result
	pi.growth = true
	pi.mu.Unlock()
	return result, nil
}

func (pi *ProcessIngress) CommitInputBodyGrowth(token uint64, capacity int64) error {
	pi.mu.Lock()
	port := pi.body
	if port == nil ||
		token == 0 ||
		token != pi.bodyToken ||
		!pi.growth ||
		pi.budgetOps == 0 {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: payload commit has no pending growth")
	}
	pi.mu.Unlock()
	err := port.CommitInputBodyGrowth(token, capacity)
	pi.mu.Lock()
	pi.growth = false
	pi.budgetOps--
	pi.idle.Broadcast()
	pi.mu.Unlock()
	return err
}

func (pi *ProcessIngress) ReleaseInputBody(token uint64) error {
	pi.mu.Lock()
	for pi.state == ProcessIngressPaused {
		pi.idle.Wait()
	}
	if pi.state == ProcessIngressContained && token != 0 && token == pi.fencedToken {
		pi.mu.Unlock()
		return nil
	}
	port := pi.body
	if pi.state != ProcessIngressLive ||
		port == nil ||
		token == 0 ||
		token != pi.bodyToken ||
		pi.growth {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: payload release has no live run binding")
	}
	pi.budgetOps++
	pi.mu.Unlock()
	err := port.ReleaseInputBody(token)
	pi.mu.Lock()
	pi.budgetOps--
	if err == nil && pi.body == port && pi.bodyToken == token {
		pi.body = nil
		pi.bodyToken = 0
	}
	pi.idle.Broadcast()
	pi.mu.Unlock()
	return err
}

func (pi *ProcessIngress) acquireDelivery() (processInputPort, bool) {
	pi.mu.Lock()
	defer pi.mu.Unlock()
	for pi.state == ProcessIngressPaused {
		pi.idle.Wait()
	}
	if pi.state != ProcessIngressLive || pi.active == nil {
		return nil, false
	}
	pi.deliveries++
	return pi.active, true
}

func (pi *ProcessIngress) releaseDelivery() {
	pi.mu.Lock()
	pi.deliveries--
	if pi.deliveries == 0 {
		pi.idle.Broadcast()
	}
	pi.mu.Unlock()
}

func (pi *ProcessIngress) dispositionDeliveryError(port processInputPort, err error) error {
	if err == nil || !errors.Is(err, jobmgr.ErrStopped) {
		return err
	}
	pi.mu.Lock()
	expected := pi.state == ProcessIngressLive && pi.pauseSealed && pi.active == port
	pi.mu.Unlock()
	if expected {
		return nil
	}
	return err
}

func (pi *ProcessIngress) discardCall(call functionwire.Call) error {
	if call.InputBodyToken == 0 {
		return nil
	}
	pi.mu.Lock()
	if pi.state == ProcessIngressContained && call.InputBodyToken == pi.fencedToken {
		pi.mu.Unlock()
		return nil
	}
	port := pi.body
	if port != nil {
		pi.body = nil
		pi.bodyToken = 0
	}
	pi.mu.Unlock()
	if port == nil {
		return errors.New("jobmgr Function process ingress: discarded payload has no run binding")
	}
	return port.ReleaseInputBody(call.InputBodyToken)
}
