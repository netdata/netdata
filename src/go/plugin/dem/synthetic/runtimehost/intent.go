// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"errors"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type transition struct {
	host           *Host
	previous, next collectorapi.JobConfigIdentity
	claim          *runtime.Claim
	consumed       bool
}

func (h *Host) Prepare(previous, next collectorapi.JobConfigIdentity, _ map[string]any) (collectorapi.PreparedJobConfigIntent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, errors.New("browser runtime owner is closed")
	}
	var claim *runtime.Claim
	if next.Valid() && h.unavailable == nil {
		var err error
		claim, err = h.store.ReserveDemand(h.manifest.Archive.SHA256)
		if err != nil {
			return nil, err
		}
	}
	t := &transition{host: h, previous: previous, next: next, claim: claim}
	h.reservations[t] = claim
	return t, nil
}

func (t *transition) Commit() {
	h := t.host
	h.mu.Lock()
	if t.consumed {
		h.mu.Unlock()
		return
	}
	t.consumed = true
	delete(h.reservations, t)
	if previous := h.accepted[t.previous]; len(previous) > 0 {
		old := previous[len(previous)-1]
		old.active = false
		if old.claim != nil {
			h.retired[old.claim] = struct{}{}
		}
		if len(previous) == 1 {
			delete(h.accepted, t.previous)
		} else {
			h.accepted[t.previous] = previous[:len(previous)-1]
		}
	}
	if t.next.Valid() {
		h.accepted[t.next] = append(h.accepted[t.next], &demand{claim: t.claim, active: true})
	}
	h.mu.Unlock()
	h.signal()
}

func (t *transition) Abort() error {
	h := t.host
	h.mu.Lock()
	if t.consumed {
		h.mu.Unlock()
		return nil
	}
	t.consumed = true
	delete(h.reservations, t)
	if t.claim != nil {
		h.retired[t.claim] = struct{}{}
	}
	h.mu.Unlock()
	if t.claim == nil {
		return nil
	}
	err := t.claim.Close()
	if err == nil {
		h.mu.Lock()
		delete(h.retired, t.claim)
		h.mu.Unlock()
	} else {
		h.signal()
	}
	return err
}
