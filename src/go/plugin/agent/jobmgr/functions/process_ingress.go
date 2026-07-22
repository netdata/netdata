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

type processInputPort interface {
	functionwire.Consumer
	Generation() uint64
}

type ProcessBinding struct {
	port processInputPort
}

func NewProcessBinding(
	kernel jobmgr.CommandPort,
	runGeneration uint64,
	clock lifecycle.Clock,
	quit func(),
) (ProcessBinding, error) {
	ingress, err := NewIngress(kernel, clock, quit)
	if err != nil {
		return ProcessBinding{}, err
	}
	if runGeneration == 0 {
		return ProcessBinding{}, errors.New("jobmgr Function process ingress: incomplete binding")
	}
	return ProcessBinding{
		port: &boundProcessInput{
			Ingress: ingress, runGeneration: runGeneration,
		},
	}, nil
}

type boundProcessInput struct {
	*Ingress
	runGeneration uint64
}

func (bpi *boundProcessInput) Generation() uint64 { return bpi.runGeneration }

// ProcessIngress owns the one process-lifetime Function reader and swaps only
// its generation-scoped delivery capability during Agent restart.
type ProcessIngress struct {
	active        processInputPort           // current generation's delivery port; nil unless live
	changed       chan struct{}              // closed and replaced on state/counter changes
	capsule       *functionwire.InputCapsule // process-lifetime wire reader (never swapped)
	state         ProcessIngressState        // paused | live | contained
	deliveries    int                        // in-flight HandleCall/Cancel/Reject/Quit count
	pauseNext     uint64                     // generation the next Adopt must match after a drain
	readerStarted bool                       // Run() guard; started exactly once
	mu            sync.Mutex                 // guards all fields
	parsing       bool                       // a returned wire read is being parsed or delivered
	waitingReads  int                        // returned reads parked while ingress is paused
	pauseSealed   bool                       // seal step done; drain may proceed
}

func NewProcessIngress(reader io.Reader) (*ProcessIngress, error) {
	if reader == nil {
		return nil, errors.New("jobmgr Function process ingress: incomplete process authority")
	}
	ingress := &ProcessIngress{
		state:   ProcessIngressPaused,
		changed: make(chan struct{}),
	}
	capsule, err := functionwire.NewInputCapsule(reader)
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
	if pi.readerStarted || pi.state == ProcessIngressContained {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: reader already started or contained")
	}
	pi.readerStarted = true
	pi.mu.Unlock()
	return pi.capsule.Run(ctx, pi)
}

func (pi *ProcessIngress) Adopt(ctx context.Context, binding ProcessBinding) error {
	if ctx == nil ||
		binding.port == nil ||
		binding.port.Generation() == 0 {
		return errors.New("jobmgr Function process ingress: invalid binding")
	}
	pi.mu.Lock()
	if pi.state != ProcessIngressPaused ||
		pi.active != nil ||
		pi.deliveries != 0 ||
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
	for pi.parsing || pi.deliveries != 0 {
		if err := pi.waitLocked(ctx); err != nil {
			pi.mu.Unlock()
			return err
		}
	}
	pi.state = ProcessIngressPaused
	pi.active = nil
	pi.pauseSealed = false
	pi.pauseNext = nextGeneration
	pi.signalLocked()
	pi.mu.Unlock()
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
		pi.parsing {
		pi.mu.Unlock()
		return errors.New("jobmgr Function process ingress: final fence outside drained pause")
	}
	pi.capsule.DiscardPausedPayload()
	pi.state = ProcessIngressContained
	pi.signalLocked()
	var fenceErr error
	for pi.waitingReads != 0 && fenceErr == nil {
		fenceErr = pi.waitLocked(ctx)
	}
	pi.mu.Unlock()

	return fenceErr
}

func (pi *ProcessIngress) State() ProcessIngressState {
	pi.mu.Lock()
	defer pi.mu.Unlock()
	return pi.state
}

// AcquireInputRead admits one returned reader segment into the parser. A read
// may already have returned when pause is sealed, so it parks here until the
// successor is adopted or the process input is contained.
func (pi *ProcessIngress) AcquireInputRead(
	ctx context.Context,
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
		return nil
	}
	defer pi.releaseDelivery()
	err := port.HandleCall(ctx, call)
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
