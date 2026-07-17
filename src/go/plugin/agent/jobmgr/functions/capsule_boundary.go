// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync"

	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

var errProcessInputContained = errors.New("Function process ingress: input capsule contained")

type capsuleBoundary struct {
	mu          sync.Mutex
	idle        chan struct{}
	idleWaiters int

	target    *ProcessIngress
	state     ProcessIngressState
	active    int
	parsing   bool
	adopting  bool
	contained bool

	readReturns    int
	waitingReads   int
	discardedReads int
}

type capsuleBoundaryCensus struct {
	ReadReturns        int
	WaitingReadReturns int
	DiscardedReads     int
	CapabilityAttached bool
}

func newCapsuleBoundary(target *ProcessIngress) (*capsuleBoundary, error) {
	if target == nil {
		return nil, errors.New("Function process ingress: nil capsule target")
	}
	return &capsuleBoundary{
		target: target,
		state:  ProcessIngressPaused,
		idle:   make(chan struct{}),
	}, nil
}

func (boundary *capsuleBoundary) signalLocked() {
	if boundary.idleWaiters == 0 {
		return
	}
	close(boundary.idle)
	boundary.idle = make(chan struct{})
	boundary.idleWaiters = 0
}

// waitLocked releases and reacquires mu. The caller holds mu on entry and exit.
func (boundary *capsuleBoundary) waitLocked(ctx context.Context) error {
	idle := boundary.idle
	boundary.idleWaiters++
	boundary.mu.Unlock()
	var err error
	select {
	case <-idle:
	case <-ctx.Done():
		err = ctx.Err()
	}
	boundary.mu.Lock()
	if idle == boundary.idle {
		boundary.idleWaiters--
	}
	return err
}

func (boundary *capsuleBoundary) SealPause() error {
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	if boundary.state != ProcessIngressLive || boundary.contained {
		return errors.New("Function process ingress: capsule pause outside live state")
	}
	boundary.state = ProcessIngressPaused
	boundary.signalLocked()
	return nil
}

func (boundary *capsuleBoundary) DrainPause(ctx context.Context) error {
	boundary.mu.Lock()
	if boundary.state != ProcessIngressPaused || boundary.contained || boundary.adopting {
		boundary.mu.Unlock()
		return errors.New("Function process ingress: capsule drain outside sealed pause")
	}
	for boundary.active != 0 || boundary.parsing {
		if err := boundary.waitLocked(ctx); err != nil {
			boundary.mu.Unlock()
			return err
		}
	}
	boundary.mu.Unlock()
	return nil
}

func (boundary *capsuleBoundary) RollbackPause() error {
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	if boundary.state != ProcessIngressPaused || boundary.contained || boundary.adopting {
		return errors.New("Function process ingress: capsule pause rollback outside pause")
	}
	boundary.state = ProcessIngressLive
	boundary.signalLocked()
	return nil
}

func (boundary *capsuleBoundary) PrepareAdopt(ctx context.Context) error {
	boundary.mu.Lock()
	if boundary.state != ProcessIngressPaused || boundary.contained || boundary.adopting {
		boundary.mu.Unlock()
		return errors.New("Function process ingress: capsule adopt preparation outside pause")
	}
	boundary.adopting = true
	boundary.signalLocked()
	for boundary.active != 0 || boundary.parsing {
		if err := boundary.waitLocked(ctx); err != nil {
			boundary.adopting = false
			boundary.signalLocked()
			boundary.mu.Unlock()
			return err
		}
	}
	boundary.mu.Unlock()
	return nil
}

func (boundary *capsuleBoundary) CommitAdopt() {
	boundary.mu.Lock()
	boundary.adopting = false
	boundary.state = ProcessIngressLive
	boundary.signalLocked()
	boundary.mu.Unlock()
}

func (boundary *capsuleBoundary) AbortAdopt() {
	boundary.mu.Lock()
	if boundary.state == ProcessIngressPaused && boundary.adopting && !boundary.contained {
		boundary.adopting = false
		boundary.signalLocked()
	}
	boundary.mu.Unlock()
}

func (boundary *capsuleBoundary) Resume() error {
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	if boundary.state != ProcessIngressPaused ||
		boundary.contained ||
		boundary.adopting ||
		boundary.active != 0 ||
		boundary.parsing {
		return errors.New("Function process ingress: capsule resume outside drained pause")
	}
	boundary.state = ProcessIngressLive
	boundary.signalLocked()
	return nil
}

func (boundary *capsuleBoundary) Fence(ctx context.Context) error {
	boundary.mu.Lock()
	if boundary.state != ProcessIngressPaused || boundary.contained || boundary.adopting {
		boundary.mu.Unlock()
		return errors.New("Function process ingress: capsule fence outside drained pause")
	}
	for boundary.active != 0 || boundary.parsing {
		if err := boundary.waitLocked(ctx); err != nil {
			boundary.mu.Unlock()
			return err
		}
	}
	boundary.target = nil
	boundary.contained = true
	boundary.state = ProcessIngressContained
	boundary.signalLocked()
	for boundary.waitingReads != 0 {
		if err := boundary.waitLocked(ctx); err != nil {
			boundary.mu.Unlock()
			return err
		}
	}
	boundary.mu.Unlock()
	return nil
}

func (boundary *capsuleBoundary) acquire() (*ProcessIngress, bool) {
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	if boundary.contained ||
		boundary.target == nil ||
		(boundary.state != ProcessIngressLive && !boundary.parsing) {
		return nil, false
	}
	boundary.active++
	return boundary.target, true
}

func (boundary *capsuleBoundary) AcquireInputRead(ctx context.Context, bufferFull bool) (bool, error) {
	boundary.mu.Lock()
	boundary.readReturns++
	if boundary.contained || boundary.target == nil {
		boundary.discardedReads++
		boundary.mu.Unlock()
		return false, nil
	}
	waiting := boundary.state == ProcessIngressPaused
	if waiting {
		boundary.waitingReads++
	}
	if (waiting || bufferFull) && !boundary.adopting {
		target := boundary.target
		boundary.active++
		boundary.mu.Unlock()
		observeErr := target.observeReadReturn()
		boundary.mu.Lock()
		boundary.active--
		boundary.signalLocked()
		boundary.mu.Unlock()
		if observeErr != nil {
			boundary.mu.Lock()
			if waiting {
				boundary.waitingReads--
			}
			boundary.signalLocked()
			boundary.mu.Unlock()
			return false, observeErr
		}
	} else {
		boundary.mu.Unlock()
	}

	boundary.mu.Lock()
	for {
		if boundary.contained {
			if waiting {
				boundary.waitingReads--
			}
			boundary.discardedReads++
			boundary.signalLocked()
			boundary.mu.Unlock()
			return false, nil
		}
		if boundary.state == ProcessIngressLive && !boundary.parsing {
			if waiting {
				boundary.waitingReads--
			}
			boundary.parsing = true
			boundary.signalLocked()
			boundary.mu.Unlock()
			return true, nil
		}
		if boundary.state != ProcessIngressPaused || boundary.parsing {
			boundary.mu.Unlock()
			return false, errors.New("Function process ingress: invalid read-return gate state")
		}
		if !waiting {
			waiting = true
			boundary.waitingReads++
			boundary.signalLocked()
		}
		if err := boundary.waitLocked(ctx); err != nil {
			if waiting {
				boundary.waitingReads--
			}
			boundary.signalLocked()
			boundary.mu.Unlock()
			return false, err
		}
	}
}

func (boundary *capsuleBoundary) Census() capsuleBoundaryCensus {
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	return capsuleBoundaryCensus{
		ReadReturns:        boundary.readReturns,
		WaitingReadReturns: boundary.waitingReads,
		DiscardedReads:     boundary.discardedReads,
		CapabilityAttached: boundary.target != nil,
	}
}

func (boundary *capsuleBoundary) ReleaseInputRead() {
	boundary.mu.Lock()
	if !boundary.parsing {
		boundary.mu.Unlock()
		return
	}
	boundary.parsing = false
	boundary.signalLocked()
	boundary.mu.Unlock()
}

func (boundary *capsuleBoundary) release() {
	boundary.mu.Lock()
	boundary.active--
	boundary.signalLocked()
	boundary.mu.Unlock()
}

func (boundary *capsuleBoundary) HandleCall(ctx context.Context, call functionwire.Call) error {
	target, ok := boundary.acquire()
	if !ok {
		return nil
	}
	defer boundary.release()
	return target.HandleCall(ctx, call)
}

func (boundary *capsuleBoundary) HandleCancel(ctx context.Context, uid string) error {
	target, ok := boundary.acquire()
	if !ok {
		return nil
	}
	defer boundary.release()
	return target.HandleCancel(ctx, uid)
}

func (boundary *capsuleBoundary) HandleReject(ctx context.Context, uid string, status int) error {
	target, ok := boundary.acquire()
	if !ok {
		return nil
	}
	defer boundary.release()
	return target.HandleReject(ctx, uid, status)
}

func (boundary *capsuleBoundary) HandleQuit(ctx context.Context) error {
	target, ok := boundary.acquire()
	if !ok {
		return nil
	}
	defer boundary.release()
	return target.HandleQuit(ctx)
}

func (boundary *capsuleBoundary) GrowInputBody(ctx context.Context, token uint64, nextCapacity int64) (uint64, error) {
	target, ok := boundary.acquire()
	if !ok {
		return 0, errProcessInputContained
	}
	defer boundary.release()
	return target.GrowInputBody(ctx, token, nextCapacity)
}

func (boundary *capsuleBoundary) CommitInputBodyGrowth(token uint64, capacity int64) error {
	target, ok := boundary.acquire()
	if !ok {
		return errProcessInputContained
	}
	defer boundary.release()
	return target.CommitInputBodyGrowth(token, capacity)
}

func (boundary *capsuleBoundary) ReleaseInputBody(token uint64) error {
	target, ok := boundary.acquire()
	if !ok {
		return nil
	}
	defer boundary.release()
	return target.ReleaseInputBody(token)
}
