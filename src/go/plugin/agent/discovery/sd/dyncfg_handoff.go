// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const sdShuttingDownMsg = "Service discovery is shutting down."

type pendingDyncfgFunction struct {
	fn        dyncfg.Function
	done      chan struct{}
	abandoned bool
	started   bool
}

type dyncfgAdmission uint8

const (
	dyncfgAdmissionAccepted dyncfgAdmission = iota + 1
	dyncfgAdmissionDuplicate
	dyncfgAdmissionClosed
)

// enqueueDyncfgFunction blocks until the function is accepted by the run loop
// and fully executed, or service discovery shuts down. The synchronous
// completion lets an owning Function transaction capture one terminal result
// and its notification batch without leaving a second response authority.
func (d *ServiceDiscovery) enqueueDyncfgFunction(fn dyncfg.Function) {
	ctx := d.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	fnCtx := dyncfgFunctionContext(fn)
	// Production captures these results inside the outer contained invocation.
	// Once fnCtx is canceled, that owner discards any racing shutdown result.
	pending, admission := d.beginDyncfg(fn)
	switch admission {
	case dyncfgAdmissionDuplicate:
		d.dyncfgApi.SendCodef(fn, 409, "A command with UID '%s' is already pending.", fn.UID())
		return
	case dyncfgAdmissionClosed:
		d.dyncfgApi.SendCodef(fn, 503, sdShuttingDownMsg)
		return
	}
	select {
	case d.dyncfgCh <- fn:
	case <-fnCtx.Done():
		d.cancelDyncfg(fn)
		return
	case <-ctx.Done():
		if d.cancelDyncfg(fn) {
			d.dyncfgApi.SendCodef(fn, 503, sdShuttingDownMsg)
		}
		return
	}
	select {
	case <-pending.done:
	case <-fnCtx.Done():
		_, started := d.abandonDyncfg(fn)
		if started {
			// The outer contained attempt owns logical timeout. Keep this
			// physical response authority until the serial command returns.
			<-pending.done
		}
	case <-ctx.Done():
		exists, started := d.abandonDyncfg(fn)
		if started {
			<-pending.done
		} else if exists {
			d.dyncfgApi.SendCodef(fn, 503, sdShuttingDownMsg)
		}
	}
}

func dyncfgFunctionContext(fn dyncfg.Function) context.Context {
	if ctx := fn.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func (d *ServiceDiscovery) beginDyncfg(
	fn dyncfg.Function,
) (pendingDyncfgFunction, dyncfgAdmission) {
	d.dyncfgMu.Lock()
	defer d.dyncfgMu.Unlock()
	if d.dyncfgClosed {
		return pendingDyncfgFunction{}, dyncfgAdmissionClosed
	}
	if fn.UID() == "" {
		done := make(chan struct{})
		close(done)
		return pendingDyncfgFunction{fn: fn, done: done},
			dyncfgAdmissionAccepted
	}
	if d.dyncfgPending == nil {
		d.dyncfgPending = make(map[string]pendingDyncfgFunction)
	}
	if _, exists := d.dyncfgPending[fn.UID()]; exists {
		return pendingDyncfgFunction{}, dyncfgAdmissionDuplicate
	}
	pending := pendingDyncfgFunction{
		fn: fn, done: make(chan struct{}),
	}
	d.dyncfgPending[fn.UID()] = pending
	return pending, dyncfgAdmissionAccepted
}

func (d *ServiceDiscovery) completeDyncfg(fn dyncfg.Function) {
	if fn.UID() == "" {
		return
	}
	d.dyncfgMu.Lock()
	pending, exists := d.dyncfgPending[fn.UID()]
	if exists {
		delete(d.dyncfgPending, fn.UID())
	}
	d.dyncfgMu.Unlock()
	if exists {
		close(pending.done)
	}
}

func (d *ServiceDiscovery) cancelDyncfg(fn dyncfg.Function) bool {
	if fn.UID() == "" {
		return true
	}
	d.dyncfgMu.Lock()
	defer d.dyncfgMu.Unlock()
	if _, exists := d.dyncfgPending[fn.UID()]; !exists {
		return false
	}
	delete(d.dyncfgPending, fn.UID())
	return true
}

// abandonDyncfg preserves a queued UID until the run loop drains it. Removing
// it here would let that stale command complete a newer request reusing the UID.
func (d *ServiceDiscovery) startDyncfg(fn dyncfg.Function) {
	if fn.UID() == "" {
		return
	}
	d.dyncfgMu.Lock()
	defer d.dyncfgMu.Unlock()
	pending, exists := d.dyncfgPending[fn.UID()]
	if !exists {
		return
	}
	pending.started = true
	d.dyncfgPending[fn.UID()] = pending
}

func (d *ServiceDiscovery) abandonDyncfg(fn dyncfg.Function) (bool, bool) {
	if fn.UID() == "" {
		return true, false
	}
	d.dyncfgMu.Lock()
	defer d.dyncfgMu.Unlock()
	pending, exists := d.dyncfgPending[fn.UID()]
	if !exists {
		return false, false
	}
	pending.abandoned = true
	d.dyncfgPending[fn.UID()] = pending
	return true, pending.started
}

func (d *ServiceDiscovery) failPendingDyncfg() {
	d.dyncfgMu.Lock()
	d.dyncfgClosed = true
	pending := make([]pendingDyncfgFunction, 0, len(d.dyncfgPending))
	for uid, command := range d.dyncfgPending {
		delete(d.dyncfgPending, uid)
		pending = append(pending, command)
	}
	d.dyncfgMu.Unlock()
	for _, command := range pending {
		if !command.abandoned && dyncfgFunctionContext(command.fn).Err() == nil {
			d.dyncfgApi.SendCodef(
				command.fn,
				503,
				sdShuttingDownMsg,
			)
		}
		close(command.done)
	}
}
