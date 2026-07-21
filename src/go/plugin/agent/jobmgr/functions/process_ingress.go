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
	State            ProcessIngressState
	ActiveDeliveries int
	BudgetOperations int
	ReaderStarts     int
	RunGeneration    uint64
	PendingBody      bool
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
	active        processInputPort           // current generation's delivery port; nil unless live
	body          processInputPort           // port owning the in-flight input body, if any
	changed       chan struct{}              // closed and replaced on state/counter changes
	capsule       *functionwire.InputCapsule // process-lifetime wire reader (never swapped)
	admission     *lifecycle.AdmissionLedger // shared admission ledger (identity-checked on Adopt)
	state         ProcessIngressState        // paused | live | contained
	runGeneration uint64                     // generation of the active binding
	deliveries    int                        // in-flight HandleCall/Cancel/Reject/Quit count
	budgetOps     int                        // in-flight body-budget operations count
	bodyToken     uint64                     // admission token of the current input body
	pauseNext     uint64                     // generation the next Adopt must match after a drain
	fencedToken   uint64                     // body token discarded by Fence (accepts late releases)
	readerStarts  int                        // Run() guard; started exactly once
	mu            sync.Mutex                 // guards all fields
	parsing       bool                       // a returned wire read is being parsed or delivered
	waitingReads  int                        // returned reads parked while ingress is paused
	bodyGrowing   bool                       // a GrowInputBody op is in flight
	pauseSealed   bool                       // seal step done; drain may proceed
	bodySuspended bool                       // body handed to admission across a generation swap
}

func NewProcessIngress(reader io.Reader, admission *lifecycle.AdmissionLedger) (*ProcessIngress, error) {
	if reader == nil || admission == nil {
		return nil, errors.New("jobmgr Function process ingress: incomplete process authority")
	}
	ingress := &ProcessIngress{
		admission: admission,
		state:     ProcessIngressPaused,
		changed:   make(chan struct{}),
	}
	capsule, err := functionwire.NewInputCapsule(reader, ingress)
	if err != nil {
		return nil, err
	}
	ingress.capsule = capsule
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
	return pi.capsule.Run(ctx, pi)
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
		pi.bodyGrowing ||
		pi.parsing {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: adopt outside drained pause")
	}
	if pi.pauseNext != 0 && binding.port.Generation() != pi.pauseNext {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: adopted generation differs from pause target")
	}
	defer pi.mu.Unlock()
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
	pi.signalLocked()
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
	pi.pauseSealed = true
	pi.signalLocked()
	pi.mu.Unlock()
	return nil
}

func (pi *ProcessIngress) DrainPause(ctx context.Context, nextGeneration uint64) error {
	if ctx == nil {
		return errors.New("jobmgr Function process ingress: nil pause context")
	}
	pi.mu.Lock()
	if pi.state != ProcessIngressLive || pi.active == nil || !pi.pauseSealed {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: drain outside sealed pause")
	}
	for pi.parsing || pi.deliveries != 0 || pi.budgetOps != 0 || pi.bodyGrowing {
		if err := pi.waitLocked(ctx); err != nil {
			pi.mu.Unlock()
			return err
		}
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
	pi.signalLocked()
	pi.mu.Unlock()
	return pauseErr
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
		pi.bodyGrowing ||
		pi.parsing {
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
	pi.state = ProcessIngressContained
	pi.runGeneration = 0
	pi.signalLocked()
	var fenceErr error
	for pi.waitingReads != 0 && fenceErr == nil {
		fenceErr = pi.waitLocked(ctx)
	}
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
	return errors.Join(discardErr, releaseErr, fenceErr)
}

func (pi *ProcessIngress) Census() ProcessIngressCensus {
	pi.mu.Lock()
	defer pi.mu.Unlock()
	return pi.censusLocked()
}

func (pi *ProcessIngress) censusLocked() ProcessIngressCensus {
	return ProcessIngressCensus{
		State:            pi.state,
		RunGeneration:    pi.runGeneration,
		ReaderStarts:     pi.readerStarts,
		ActiveDeliveries: pi.deliveries,
		BudgetOperations: pi.budgetOps,
		PendingBody:      pi.body != nil || pi.bodySuspended,
	}
}

// AcquireInputRead admits one returned reader segment into the parser. A read
// may already have returned when pause is sealed, so it parks here until the
// successor is adopted or the process input is contained.
func (pi *ProcessIngress) AcquireInputRead(
	ctx context.Context,
	_ bool,
) (bool, error) {
	if ctx == nil {
		return false, errors.New("jobmgr Function process ingress: nil read context")
	}
	pi.mu.Lock()
	waiting := false
	for {
		if pi.state == ProcessIngressContained {
			if waiting {
				pi.waitingReads--
				pi.signalLocked()
			}
			pi.mu.Unlock()
			return false, nil
		}
		if pi.state == ProcessIngressLive && !pi.pauseSealed && !pi.parsing {
			if waiting {
				pi.waitingReads--
			}
			pi.parsing = true
			pi.signalLocked()
			pi.mu.Unlock()
			return true, nil
		}
		paused := pi.state == ProcessIngressPaused ||
			pi.state == ProcessIngressLive && pi.pauseSealed
		if !paused {
			pi.mu.Unlock()
			return false, errors.New("jobmgr Function process ingress: invalid read-return gate state")
		}
		if !waiting {
			waiting = true
			pi.waitingReads++
			pi.signalLocked()
		}
		if err := pi.waitLocked(ctx); err != nil {
			pi.waitingReads--
			pi.signalLocked()
			pi.mu.Unlock()
			return false, err
		}
	}
}

func (pi *ProcessIngress) ReleaseInputRead() {
	pi.mu.Lock()
	if pi.parsing {
		pi.parsing = false
		pi.signalLocked()
	}
	pi.mu.Unlock()
}

func (pi *ProcessIngress) signalLocked() {
	close(pi.changed)
	pi.changed = make(chan struct{})
}

// waitLocked releases and reacquires mu while waiting for an ingress change.
func (pi *ProcessIngress) waitLocked(ctx context.Context) error {
	changed := pi.changed
	pi.mu.Unlock()
	var err error
	select {
	case <-changed:
	case <-ctx.Done():
		err = ctx.Err()
	}
	pi.mu.Lock()
	return err
}

func (pi *ProcessIngress) HandleCall(ctx context.Context, call functionwire.Call) error {
	port, ok := pi.acquireDelivery(ctx)
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
	port, ok := pi.acquireDelivery(ctx)
	if !ok {
		return nil
	}
	defer pi.releaseDelivery()
	return pi.dispositionDeliveryError(port, port.HandleCancel(ctx, uid))
}

func (pi *ProcessIngress) HandleReject(ctx context.Context, uid string, status int) error {
	port, ok := pi.acquireDelivery(ctx)
	if !ok {
		return nil
	}
	defer pi.releaseDelivery()
	return pi.dispositionDeliveryError(port, port.HandleReject(ctx, uid, status))
}

func (pi *ProcessIngress) HandleQuit(ctx context.Context) error {
	port, ok := pi.acquireDelivery(ctx)
	if !ok {
		return nil
	}
	defer pi.releaseDelivery()
	return port.HandleQuit(ctx)
}

func (pi *ProcessIngress) GrowInputBody(ctx context.Context, token uint64, nextCapacity int64) (uint64, error) {
	pi.mu.Lock()
	for pi.state == ProcessIngressPaused {
		if err := pi.waitLocked(ctx); err != nil {
			pi.mu.Unlock()
			return 0, err
		}
	}
	if pi.state != ProcessIngressLive || pi.active == nil || pi.bodyGrowing {
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
		pi.signalLocked()
		pi.mu.Unlock()
		return 0, err
	}
	pi.bodyToken = result
	pi.bodyGrowing = true
	pi.mu.Unlock()
	return result, nil
}

func (pi *ProcessIngress) CommitInputBodyGrowth(token uint64, capacity int64) error {
	pi.mu.Lock()
	port := pi.body
	if port == nil ||
		token == 0 ||
		token != pi.bodyToken ||
		!pi.bodyGrowing ||
		pi.budgetOps == 0 {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: payload commit has no pending growth")
	}
	pi.mu.Unlock()
	err := port.CommitInputBodyGrowth(token, capacity)
	pi.mu.Lock()
	pi.bodyGrowing = false
	pi.budgetOps--
	pi.signalLocked()
	pi.mu.Unlock()
	return err
}

func (pi *ProcessIngress) ReleaseInputBody(token uint64) error {
	pi.mu.Lock()
	for pi.state == ProcessIngressPaused {
		_ = pi.waitLocked(context.Background())
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
		pi.bodyGrowing {
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
	pi.signalLocked()
	pi.mu.Unlock()
	return err
}

func (pi *ProcessIngress) acquireDelivery(
	ctx context.Context,
) (processInputPort, bool) {
	pi.mu.Lock()
	defer pi.mu.Unlock()
	for pi.state == ProcessIngressPaused {
		if err := pi.waitLocked(ctx); err != nil {
			return nil, false
		}
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
		pi.signalLocked()
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
	pi.mu.Unlock()
	return errors.New("jobmgr Function process ingress: discarded payload has no run binding")
}
