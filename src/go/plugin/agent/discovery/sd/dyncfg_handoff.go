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

// enqueueDyncfgFunction blocks until the function is accepted by the run loop
// and fully executed, or service discovery shuts down. The synchronous
// completion lets an owning Function transaction capture one terminal result
// and its notification batch without leaving a second response authority.
func (d *ServiceDiscovery) enqueueDyncfgFunction(fn dyncfg.Function) {
	ctx := d.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	pending, ok := d.beginDyncfg(fn)
	if !ok {
		d.dyncfgApi.SendCodef(
			fn,
			409,
			"A command with UID '%s' is already pending.",
			fn.UID(),
		)
		return
	}
	select {
	case d.dyncfgCh <- fn:
		<-pending.done
	case <-ctx.Done():
		d.cancelDyncfg(fn)
		d.dyncfgApi.SendCodef(fn, 503, sdShuttingDownMsg)
	}
}

func (d *ServiceDiscovery) beginDyncfg(
	fn dyncfg.Function,
) (pendingDyncfgFunction, bool) {
	if fn.UID() == "" {
		done := make(chan struct{})
		close(done)
		return pendingDyncfgFunction{fn: fn, done: done}, true
	}
	d.dyncfgMu.Lock()
	defer d.dyncfgMu.Unlock()
	if d.dyncfgPending == nil {
		d.dyncfgPending = make(map[string]pendingDyncfgFunction)
	}
	if _, exists := d.dyncfgPending[fn.UID()]; exists {
		return pendingDyncfgFunction{}, false
	}
	pending := pendingDyncfgFunction{
		fn: fn, done: make(chan struct{}),
	}
	d.dyncfgPending[fn.UID()] = pending
	return pending, true
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

func (d *ServiceDiscovery) cancelDyncfg(fn dyncfg.Function) {
	if fn.UID() == "" {
		return
	}
	d.dyncfgMu.Lock()
	delete(d.dyncfgPending, fn.UID())
	d.dyncfgMu.Unlock()
}

func (d *ServiceDiscovery) failPendingDyncfg() {
	d.dyncfgMu.Lock()
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
