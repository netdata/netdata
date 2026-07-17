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
	RunGeneration          uint64
	ReaderStarts           int
	ReadReturns            int
	WaitingReadReturns     int
	DiscardedReads         int
	CapabilityAttached     bool
	CapsulePayloadActive   bool
	CapsulePayloadBytes    int
	CapsulePayloadCapacity int
	CapsuleDiscardingLine  bool
	ActiveDeliveries       int
	BudgetOperations       int
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
		return ProcessBinding{}, errors.New("Function process ingress: incomplete binding")
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

func (input *boundProcessInput) Generation() uint64 { return input.runGeneration }

func (input *boundProcessInput) SuspendInputBody(nextGeneration, token uint64) error {
	if err := input.admission.SuspendInputBody(input.runGeneration, nextGeneration, token); err != nil {
		return err
	}
	input.inputBodyBudget.kernel.NotifyControlReady()
	return nil
}

func (input *boundProcessInput) AdoptInputBody(token uint64) error {
	return input.admission.AdoptInputBody(input.runGeneration, token)
}

// ProcessIngress owns the one process-lifetime Function reader and swaps only
// its generation-scoped delivery capability during Agent restart.
type ProcessIngress struct {
	mu   sync.Mutex
	idle *sync.Cond

	capsule       *functionwire.InputCapsule
	boundary      *capsuleBoundary
	admission     *lifecycle.AdmissionLedger
	state         ProcessIngressState
	active        processInputPort
	body          processInputPort
	bodySuspended bool

	readerStarts  int
	runGeneration uint64
	deliveries    int
	budgetOps     int
	growth        bool
	bodyToken     uint64
	pauseSealed   bool
	pauseNext     uint64
	fencedToken   uint64
	observer      func(ProcessIngressCensus) error
	readObserver  func(ProcessIngressCensus) error
}

func NewProcessIngress(reader io.Reader, admission *lifecycle.AdmissionLedger) (*ProcessIngress, error) {
	if reader == nil || admission == nil {
		return nil, errors.New("Function process ingress: incomplete process authority")
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

func (ingress *ProcessIngress) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Function process ingress: nil reader context")
	}
	ingress.mu.Lock()
	if ingress.readerStarts != 0 || ingress.state == ProcessIngressContained {
		ingress.mu.Unlock()
		return errors.New("Function process ingress: reader already started or contained")
	}
	ingress.readerStarts++
	ingress.mu.Unlock()
	return ingress.capsule.Run(ctx, ingress.boundary)
}

// SetObserver installs a pre-reader test/diagnostic observer for process input
// state transitions.
func (ingress *ProcessIngress) SetObserver(observer func(ProcessIngressCensus) error) error {
	if observer == nil {
		return errors.New("Function process ingress: nil observer")
	}
	ingress.mu.Lock()
	defer ingress.mu.Unlock()
	if ingress.readerStarts != 0 || ingress.observer != nil {
		return errors.New("Function process ingress: observer changed after reader start")
	}
	ingress.observer = observer
	return nil
}

// SetReadReturnObserver installs a pre-reader test/diagnostic observer for the
// process read-return boundary.
func (ingress *ProcessIngress) SetReadReturnObserver(observer func(ProcessIngressCensus) error) error {
	if observer == nil {
		return errors.New("Function process ingress: nil read-return observer")
	}
	ingress.mu.Lock()
	defer ingress.mu.Unlock()
	if ingress.readerStarts != 0 || ingress.readObserver != nil {
		return errors.New("Function process ingress: read-return observer changed after reader start")
	}
	ingress.readObserver = observer
	return nil
}

func (ingress *ProcessIngress) Adopt(ctx context.Context, binding ProcessBinding) error {
	if ctx == nil ||
		binding.port == nil ||
		binding.admission == nil ||
		binding.admission != ingress.admission ||
		binding.port.Generation() == 0 {
		return errors.New("Function process ingress: invalid binding")
	}
	ingress.mu.Lock()
	if ingress.state != ProcessIngressPaused ||
		ingress.active != nil ||
		ingress.deliveries != 0 ||
		ingress.budgetOps != 0 ||
		ingress.growth {
		ingress.mu.Unlock()
		return errors.New("Function process ingress: adopt outside drained pause")
	}
	if ingress.pauseNext != 0 && binding.port.Generation() != ingress.pauseNext {
		ingress.mu.Unlock()
		return errors.New("Function process ingress: adopted generation differs from pause target")
	}
	ingress.mu.Unlock()
	if err := ingress.boundary.PrepareAdopt(ctx); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			ingress.boundary.AbortAdopt()
		}
	}()
	ingress.mu.Lock()
	defer ingress.mu.Unlock()
	if ingress.state != ProcessIngressPaused ||
		ingress.active != nil ||
		ingress.deliveries != 0 ||
		ingress.budgetOps != 0 ||
		ingress.growth {
		return errors.New("Function process ingress: adopt state changed during preparation")
	}
	ingress.active = binding.port
	previousGeneration := ingress.runGeneration
	ingress.runGeneration = binding.port.Generation()
	if ingress.body != nil || ingress.bodySuspended {
		if ingress.bodyToken == 0 {
			ingress.active = nil
			ingress.runGeneration = previousGeneration
			return errors.New("Function process ingress: adopted body has no token")
		}
		if ingress.body != nil || !ingress.bodySuspended {
			ingress.active = nil
			ingress.runGeneration = previousGeneration
			return errors.New("Function process ingress: adopted body retained its old run binding")
		}
		if err := binding.port.AdoptInputBody(ingress.bodyToken); err != nil {
			ingress.active = nil
			ingress.runGeneration = previousGeneration
			return err
		}
		ingress.body = binding.port
		ingress.bodySuspended = false
	}
	ingress.pauseNext = 0
	ingress.state = ProcessIngressLive
	ingress.boundary.CommitAdopt()
	committed = true
	ingress.idle.Broadcast()
	return nil
}

func (ingress *ProcessIngress) SealPause() error {
	ingress.mu.Lock()
	if ingress.state != ProcessIngressLive || ingress.active == nil {
		ingress.mu.Unlock()
		return errors.New("Function process ingress: pause outside live state")
	}
	if ingress.pauseSealed {
		ingress.mu.Unlock()
		return nil
	}
	ingress.mu.Unlock()
	if err := ingress.boundary.SealPause(); err != nil {
		return err
	}
	ingress.mu.Lock()
	if ingress.state != ProcessIngressLive || ingress.active == nil || ingress.pauseSealed {
		ingress.mu.Unlock()
		_ = ingress.boundary.RollbackPause()
		return errors.New("Function process ingress: state changed while sealing pause")
	}
	ingress.pauseSealed = true
	ingress.mu.Unlock()
	return nil
}

func (ingress *ProcessIngress) DrainPause(ctx context.Context, nextGeneration uint64) error {
	if ctx == nil {
		return errors.New("Function process ingress: nil pause context")
	}
	ingress.mu.Lock()
	valid := ingress.state == ProcessIngressLive && ingress.active != nil && ingress.pauseSealed
	ingress.mu.Unlock()
	if !valid {
		return errors.New("Function process ingress: drain outside sealed pause")
	}
	if err := ingress.boundary.DrainPause(ctx); err != nil {
		return err
	}
	ingress.mu.Lock()
	if ingress.state != ProcessIngressLive ||
		ingress.active == nil ||
		ingress.deliveries != 0 ||
		ingress.budgetOps != 0 ||
		ingress.growth {
		ingress.state = ProcessIngressPaused
		ingress.active = nil
		ingress.pauseSealed = false
		ingress.pauseNext = 0
		ingress.idle.Broadcast()
		ingress.mu.Unlock()
		return errors.New("Function process ingress: pause did not drain process input")
	}
	var pauseErr error
	if ingress.body != nil {
		if ingress.bodyToken == 0 {
			pauseErr = errors.New("Function process ingress: partial payload has no admission token")
		} else {
			pauseErr = ingress.body.SuspendInputBody(nextGeneration, ingress.bodyToken)
			if pauseErr == nil {
				ingress.body = nil
				ingress.bodySuspended = true
			}
		}
	} else if ingress.bodySuspended {
		pauseErr = errors.New("Function process ingress: body was already suspended")
	}
	ingress.state = ProcessIngressPaused
	ingress.active = nil
	ingress.pauseSealed = false
	if pauseErr == nil {
		ingress.pauseNext = nextGeneration
	} else {
		ingress.pauseNext = 0
	}
	ingress.idle.Broadcast()
	ingress.mu.Unlock()
	return pauseErr
}

func (ingress *ProcessIngress) Pause(ctx context.Context, nextGeneration uint64) error {
	if err := ingress.SealPause(); err != nil {
		return err
	}
	if err := ingress.DrainPause(ctx, nextGeneration); err != nil {
		ingress.mu.Lock()
		rollback := ingress.state == ProcessIngressLive && ingress.pauseSealed
		if rollback {
			ingress.pauseSealed = false
		}
		ingress.mu.Unlock()
		if rollback {
			_ = ingress.boundary.RollbackPause()
		}
		return err
	}
	return nil
}

func (ingress *ProcessIngress) Fence(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Function process ingress: nil fence context")
	}
	ingress.mu.Lock()
	if ingress.state != ProcessIngressPaused ||
		ingress.active != nil ||
		ingress.deliveries != 0 ||
		ingress.budgetOps != 0 ||
		ingress.growth {
		ingress.mu.Unlock()
		return errors.New("Function process ingress: final fence outside drained pause")
	}
	body := ingress.body
	bodySuspended := ingress.bodySuspended
	token := ingress.bodyToken
	discardErr := ingress.capsule.DiscardPausedPayload(token)
	ingress.body = nil
	ingress.bodySuspended = false
	ingress.bodyToken = 0
	ingress.fencedToken = token
	ingress.observer = nil
	ingress.readObserver = nil
	ingress.idle.Broadcast()
	ingress.mu.Unlock()

	var releaseErr error
	if body != nil {
		releaseErr = body.ReleaseInputBody(token)
	} else if bodySuspended {
		wake, err := ingress.admission.AbortInputBody(token)
		releaseErr = err
		if err == nil && wake {
			releaseErr = errors.New("Function process ingress: suspended body release exposed unrelated grantable work")
		}
	}
	fenceErr := ingress.boundary.Fence(ctx)
	boundary := ingress.boundary.Census()
	if !boundary.CapabilityAttached {
		ingress.mu.Lock()
		ingress.state = ProcessIngressContained
		ingress.runGeneration = 0
		ingress.idle.Broadcast()
		ingress.mu.Unlock()
	}
	return errors.Join(discardErr, releaseErr, fenceErr)
}

func (ingress *ProcessIngress) Census() ProcessIngressCensus {
	ingress.mu.Lock()
	defer ingress.mu.Unlock()
	return ingress.censusLocked()
}

func (ingress *ProcessIngress) censusLocked() ProcessIngressCensus {
	boundary := ingress.boundary.Census()
	var contained functionwire.ContainedInputCensus
	if ingress.state == ProcessIngressContained {
		contained = ingress.capsule.ContainedCensus()
	}
	return ProcessIngressCensus{
		State:                  ingress.state,
		RunGeneration:          ingress.runGeneration,
		ReaderStarts:           ingress.readerStarts,
		ReadReturns:            boundary.ReadReturns,
		WaitingReadReturns:     boundary.WaitingReadReturns,
		DiscardedReads:         boundary.DiscardedReads,
		CapabilityAttached:     boundary.CapabilityAttached,
		CapsulePayloadActive:   contained.PayloadActive,
		CapsulePayloadBytes:    contained.PayloadBytes,
		CapsulePayloadCapacity: contained.PayloadCapacity,
		CapsuleDiscardingLine:  contained.DiscardingLine,
		ActiveDeliveries:       ingress.deliveries,
		BudgetOperations:       ingress.budgetOps,
		PendingBody:            ingress.body != nil || ingress.bodySuspended,
		BodyBindingAttached:    ingress.body != nil,
		BodySuspended:          ingress.bodySuspended,
	}
}

func (ingress *ProcessIngress) observeReadReturn() error {
	ingress.mu.Lock()
	observer := ingress.readObserver
	census := ingress.censusLocked()
	ingress.mu.Unlock()
	if observer == nil {
		return nil
	}
	return observer(census)
}

func (ingress *ProcessIngress) HandleCall(ctx context.Context, call functionwire.Call) error {
	port, ok := ingress.acquireDelivery()
	if !ok {
		return ingress.discardCall(call)
	}
	defer ingress.releaseDelivery()
	err := port.HandleCall(ctx, call)
	if call.InputBodyToken != 0 {
		ingress.mu.Lock()
		if ingress.body == port {
			ingress.body = nil
			ingress.bodyToken = 0
		}
		ingress.mu.Unlock()
	}
	return ingress.dispositionDeliveryError(port, err)
}

func (ingress *ProcessIngress) HandleCancel(ctx context.Context, uid string) error {
	port, ok := ingress.acquireDelivery()
	if !ok {
		return nil
	}
	defer ingress.releaseDelivery()
	return ingress.dispositionDeliveryError(port, port.HandleCancel(ctx, uid))
}

func (ingress *ProcessIngress) HandleReject(ctx context.Context, uid string, status int) error {
	port, ok := ingress.acquireDelivery()
	if !ok {
		return nil
	}
	defer ingress.releaseDelivery()
	return ingress.dispositionDeliveryError(port, port.HandleReject(ctx, uid, status))
}

func (ingress *ProcessIngress) HandleQuit(ctx context.Context) error {
	port, ok := ingress.acquireDelivery()
	if !ok {
		return nil
	}
	defer ingress.releaseDelivery()
	return port.HandleQuit(ctx)
}

func (ingress *ProcessIngress) GrowInputBody(ctx context.Context, token uint64, nextCapacity int64) (uint64, error) {
	ingress.mu.Lock()
	for ingress.state == ProcessIngressPaused {
		ingress.idle.Wait()
	}
	if ingress.state != ProcessIngressLive || ingress.active == nil || ingress.growth {
		ingress.mu.Unlock()
		return 0, errors.New("Function process ingress: payload growth outside live state")
	}
	port := ingress.body
	if token == 0 {
		if ingress.body != nil || ingress.bodySuspended {
			ingress.mu.Unlock()
			return 0, errors.New("Function process ingress: payload started outside live state")
		}
		port = ingress.active
		ingress.body = port
	} else if port == nil || token != ingress.bodyToken {
		ingress.mu.Unlock()
		return 0, errors.New("Function process ingress: stale payload growth")
	}
	ingress.budgetOps++
	ingress.mu.Unlock()
	result, err := port.GrowInputBody(ctx, token, nextCapacity)
	ingress.mu.Lock()
	if err != nil {
		ingress.budgetOps--
		if token == 0 && ingress.body == port {
			ingress.body = nil
		}
		ingress.idle.Broadcast()
		ingress.mu.Unlock()
		return 0, err
	}
	ingress.bodyToken = result
	ingress.growth = true
	ingress.mu.Unlock()
	return result, nil
}

func (ingress *ProcessIngress) CommitInputBodyGrowth(token uint64, capacity int64) error {
	ingress.mu.Lock()
	port := ingress.body
	if port == nil ||
		token == 0 ||
		token != ingress.bodyToken ||
		!ingress.growth ||
		ingress.budgetOps == 0 {
		ingress.mu.Unlock()
		return errors.New("Function process ingress: payload commit has no pending growth")
	}
	ingress.mu.Unlock()
	err := port.CommitInputBodyGrowth(token, capacity)
	ingress.mu.Lock()
	ingress.growth = false
	ingress.budgetOps--
	census := ingress.censusLocked()
	observer := ingress.observer
	ingress.idle.Broadcast()
	ingress.mu.Unlock()
	if err == nil && observer != nil {
		err = observer(census)
	}
	return err
}

func (ingress *ProcessIngress) ReleaseInputBody(token uint64) error {
	ingress.mu.Lock()
	for ingress.state == ProcessIngressPaused {
		ingress.idle.Wait()
	}
	if ingress.state == ProcessIngressContained && token != 0 && token == ingress.fencedToken {
		ingress.mu.Unlock()
		return nil
	}
	port := ingress.body
	if ingress.state != ProcessIngressLive ||
		port == nil ||
		token == 0 ||
		token != ingress.bodyToken ||
		ingress.growth {
		ingress.mu.Unlock()
		return errors.New("Function process ingress: payload release has no live run binding")
	}
	ingress.budgetOps++
	ingress.mu.Unlock()
	err := port.ReleaseInputBody(token)
	ingress.mu.Lock()
	ingress.budgetOps--
	if err == nil && ingress.body == port && ingress.bodyToken == token {
		ingress.body = nil
		ingress.bodyToken = 0
	}
	ingress.idle.Broadcast()
	ingress.mu.Unlock()
	return err
}

func (ingress *ProcessIngress) acquireDelivery() (processInputPort, bool) {
	ingress.mu.Lock()
	defer ingress.mu.Unlock()
	for ingress.state == ProcessIngressPaused {
		ingress.idle.Wait()
	}
	if ingress.state != ProcessIngressLive || ingress.active == nil {
		return nil, false
	}
	ingress.deliveries++
	return ingress.active, true
}

func (ingress *ProcessIngress) releaseDelivery() {
	ingress.mu.Lock()
	ingress.deliveries--
	if ingress.deliveries == 0 {
		ingress.idle.Broadcast()
	}
	ingress.mu.Unlock()
}

func (ingress *ProcessIngress) dispositionDeliveryError(port processInputPort, err error) error {
	if err == nil || !errors.Is(err, jobmgr.ErrStopped) {
		return err
	}
	ingress.mu.Lock()
	expected := ingress.state == ProcessIngressLive && ingress.pauseSealed && ingress.active == port
	ingress.mu.Unlock()
	if expected {
		return nil
	}
	return err
}

func (ingress *ProcessIngress) discardCall(call functionwire.Call) error {
	if call.InputBodyToken == 0 {
		return nil
	}
	ingress.mu.Lock()
	if ingress.state == ProcessIngressContained && call.InputBodyToken == ingress.fencedToken {
		ingress.mu.Unlock()
		return nil
	}
	port := ingress.body
	if port != nil {
		ingress.body = nil
		ingress.bodyToken = 0
	}
	ingress.mu.Unlock()
	if port == nil {
		return errors.New("Function process ingress: discarded payload has no run binding")
	}
	return port.ReleaseInputBody(call.InputBodyToken)
}
