// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const sdShuttingDownMsg = "Service discovery is shutting down."

type pendingDyncfgFunction struct {
	fn   dyncfg.Function
	done chan struct{}
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
		<-pending.done
	case <-ctx.Done():
		if d.cancelDyncfg(fn) {
			d.dyncfgApi.SendCodef(fn, 503, sdShuttingDownMsg)
		}
	}
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
		d.dyncfgApi.SendCodef(
			command.fn,
			503,
			sdShuttingDownMsg,
		)
		close(command.done)
	}
}
