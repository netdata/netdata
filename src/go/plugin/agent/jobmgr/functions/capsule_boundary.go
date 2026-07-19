// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync"

	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

var errProcessInputContained = errors.New("jobmgr Function process ingress: input capsule contained")

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
		return nil, errors.New("jobmgr Function process ingress: nil capsule target")
	}
	return &capsuleBoundary{
		target: target,
		state:  ProcessIngressPaused,
		idle:   make(chan struct{}),
	}, nil
}

func (cb *capsuleBoundary) signalLocked() {
	if cb.idleWaiters == 0 {
		return
	}
	close(cb.idle)
	cb.idle = make(chan struct{})
	cb.idleWaiters = 0
}

// waitLocked releases and reacquires mu. The caller holds mu on entry and exit.
func (cb *capsuleBoundary) waitLocked(ctx context.Context) error {
	idle := cb.idle
	cb.idleWaiters++
	cb.mu.Unlock()
	var err error
	select {
	case <-idle:
	case <-ctx.Done():
		err = ctx.Err()
	}
	cb.mu.Lock()
	if idle == cb.idle {
		cb.idleWaiters--
	}
	return err
}

func (cb *capsuleBoundary) sealPause() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state != ProcessIngressLive || cb.contained {
		return errors.New("jobmgr Function process ingress: capsule pause outside live state")
	}
	cb.state = ProcessIngressPaused
	cb.signalLocked()
	return nil
}

func (cb *capsuleBoundary) drainPause(ctx context.Context) error {
	cb.mu.Lock()
	if cb.state != ProcessIngressPaused || cb.contained || cb.adopting {
		cb.mu.Unlock()
		return errors.New("jobmgr Function process ingress: capsule drain outside sealed pause")
	}
	for cb.active != 0 || cb.parsing {
		if err := cb.waitLocked(ctx); err != nil {
			cb.mu.Unlock()
			return err
		}
	}
	cb.mu.Unlock()
	return nil
}

func (cb *capsuleBoundary) rollbackPause() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state != ProcessIngressPaused || cb.contained || cb.adopting {
		return errors.New("jobmgr Function process ingress: capsule pause rollback outside pause")
	}
	cb.state = ProcessIngressLive
	cb.signalLocked()
	return nil
}

func (cb *capsuleBoundary) prepareAdopt(ctx context.Context) error {
	cb.mu.Lock()
	if cb.state != ProcessIngressPaused || cb.contained || cb.adopting {
		cb.mu.Unlock()
		return errors.New("jobmgr Function process ingress: capsule adopt preparation outside pause")
	}
	cb.adopting = true
	cb.signalLocked()
	for cb.active != 0 || cb.parsing {
		if err := cb.waitLocked(ctx); err != nil {
			cb.adopting = false
			cb.signalLocked()
			cb.mu.Unlock()
			return err
		}
	}
	cb.mu.Unlock()
	return nil
}

func (cb *capsuleBoundary) commitAdopt() {
	cb.mu.Lock()
	cb.adopting = false
	cb.state = ProcessIngressLive
	cb.signalLocked()
	cb.mu.Unlock()
}

func (cb *capsuleBoundary) abortAdopt() {
	cb.mu.Lock()
	if cb.state == ProcessIngressPaused && cb.adopting && !cb.contained {
		cb.adopting = false
		cb.signalLocked()
	}
	cb.mu.Unlock()
}

func (cb *capsuleBoundary) fence(ctx context.Context) error {
	cb.mu.Lock()
	if cb.state != ProcessIngressPaused || cb.contained || cb.adopting {
		cb.mu.Unlock()
		return errors.New("jobmgr Function process ingress: capsule fence outside drained pause")
	}
	for cb.active != 0 || cb.parsing {
		if err := cb.waitLocked(ctx); err != nil {
			cb.mu.Unlock()
			return err
		}
	}
	cb.target = nil
	cb.contained = true
	cb.state = ProcessIngressContained
	cb.signalLocked()
	for cb.waitingReads != 0 {
		if err := cb.waitLocked(ctx); err != nil {
			cb.mu.Unlock()
			return err
		}
	}
	cb.mu.Unlock()
	return nil
}

func (cb *capsuleBoundary) acquire() (*ProcessIngress, bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.contained ||
		cb.target == nil ||
		(cb.state != ProcessIngressLive && !cb.parsing) {
		return nil, false
	}
	cb.active++
	return cb.target, true
}

func (cb *capsuleBoundary) AcquireInputRead(ctx context.Context, _ bool) (bool, error) {
	cb.mu.Lock()
	cb.readReturns++
	if cb.contained || cb.target == nil {
		cb.discardedReads++
		cb.mu.Unlock()
		return false, nil
	}
	waiting := cb.state == ProcessIngressPaused
	if waiting {
		cb.waitingReads++
	}
	for {
		if cb.contained {
			if waiting {
				cb.waitingReads--
			}
			cb.discardedReads++
			cb.signalLocked()
			cb.mu.Unlock()
			return false, nil
		}
		if cb.state == ProcessIngressLive && !cb.parsing {
			if waiting {
				cb.waitingReads--
			}
			cb.parsing = true
			cb.signalLocked()
			cb.mu.Unlock()
			return true, nil
		}
		if cb.state != ProcessIngressPaused || cb.parsing {
			cb.mu.Unlock()
			return false, errors.New("jobmgr Function process ingress: invalid read-return gate state")
		}
		if !waiting {
			waiting = true
			cb.waitingReads++
			cb.signalLocked()
		}
		if err := cb.waitLocked(ctx); err != nil {
			if waiting {
				cb.waitingReads--
			}
			cb.signalLocked()
			cb.mu.Unlock()
			return false, err
		}
	}
}

func (cb *capsuleBoundary) census() capsuleBoundaryCensus {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return capsuleBoundaryCensus{
		ReadReturns:        cb.readReturns,
		WaitingReadReturns: cb.waitingReads,
		DiscardedReads:     cb.discardedReads,
		CapabilityAttached: cb.target != nil,
	}
}

func (cb *capsuleBoundary) ReleaseInputRead() {
	cb.mu.Lock()
	if !cb.parsing {
		cb.mu.Unlock()
		return
	}
	cb.parsing = false
	cb.signalLocked()
	cb.mu.Unlock()
}

func (cb *capsuleBoundary) release() {
	cb.mu.Lock()
	cb.active--
	cb.signalLocked()
	cb.mu.Unlock()
}

func (cb *capsuleBoundary) HandleCall(ctx context.Context, call functionwire.Call) error {
	target, ok := cb.acquire()
	if !ok {
		return nil
	}
	defer cb.release()
	return target.HandleCall(ctx, call)
}

func (cb *capsuleBoundary) HandleCancel(ctx context.Context, uid string) error {
	target, ok := cb.acquire()
	if !ok {
		return nil
	}
	defer cb.release()
	return target.HandleCancel(ctx, uid)
}

func (cb *capsuleBoundary) HandleReject(ctx context.Context, uid string, status int) error {
	target, ok := cb.acquire()
	if !ok {
		return nil
	}
	defer cb.release()
	return target.HandleReject(ctx, uid, status)
}

func (cb *capsuleBoundary) HandleQuit(ctx context.Context) error {
	target, ok := cb.acquire()
	if !ok {
		return nil
	}
	defer cb.release()
	return target.HandleQuit(ctx)
}

func (cb *capsuleBoundary) GrowInputBody(ctx context.Context, token uint64, nextCapacity int64) (uint64, error) {
	target, ok := cb.acquire()
	if !ok {
		return 0, errProcessInputContained
	}
	defer cb.release()
	return target.GrowInputBody(ctx, token, nextCapacity)
}

func (cb *capsuleBoundary) CommitInputBodyGrowth(token uint64, capacity int64) error {
	target, ok := cb.acquire()
	if !ok {
		return errProcessInputContained
	}
	defer cb.release()
	return target.CommitInputBodyGrowth(token, capacity)
}

func (cb *capsuleBoundary) ReleaseInputBody(token uint64) error {
	target, ok := cb.acquire()
	if !ok {
		return nil
	}
	defer cb.release()
	return target.ReleaseInputBody(token)
}
