// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"slices"
	"time"
)

// measurement is one series detached for publication: its counter or gauge
// value and its finished observation window.
type measurement struct {
	owner  *series
	value  float64
	window *interval
}

// receiverStats are cumulative receiver diagnostics captured with a cut.
type receiverStats struct {
	accepted uint64
	rejects  [len(rejectReasons)]uint64
	series   int
}

// cutResult is one coherent handoff: the batch, the profile membership captured
// with the input that activated it, and receiver diagnostics.
type cutResult struct {
	batch      []measurement
	membership []bool // nil when unchanged since the caller's last published count
	activated  int
	stats      receiverStats
}

// cut detaches the batch. published is the activation count the caller last
// captured in its templates; a different count returns a membership copy.
func (r *receiver) cut(now time.Time, published int) (cutResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return cutResult{}, rejectUnavailable
	}
	// Only a later Collect knows whether the previous staged cycle committed.
	// Incoming traffic must retain its metadata authority until this point.
	r.reconcileMetadata()
	batch := make([]measurement, 0, len(r.entries))
	r.retireAt = time.Time{}
	for _, e := range r.entries {
		expired := r.expired(e, now)
		if !expired || e.pending {
			e.meta.refs++
			batch = append(batch, measurement{e, e.value, e.window})
			e.window, e.spare = e.spare, nil
			e.pending = false
		}
		if expired {
			r.remove(e)
		} else if r.idle > 0 {
			r.boundRetirement(e)
		}
	}
	result := cutResult{
		batch:     batch,
		activated: r.activated,
		stats: receiverStats{
			accepted: r.accepted,
			rejects:  r.rejects,
			series:   len(r.entries),
		},
	}
	if r.activated != published {
		result.membership = slices.Clone(r.membership)
	}
	return result, nil
}

// release returns a published batch. Detached windows are cleared before the
// lock is taken: new input continues in the other window meanwhile.
func (r *receiver) release(batch []measurement) {
	for _, m := range batch {
		if m.window != nil {
			m.window.reset()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range batch {
		e := m.owner
		e.meta.staged = true
		e.meta.refs--
		if r.entries[e.id] == e {
			e.spare = m.window
		}
	}
}

// reconcileMetadata resolves whether the previous staged write committed, then
// retires unreferenced declarations that are uncommitted or past the horizon.
func (r *receiver) reconcileMetadata() {
	success := r.retention.SuccessfulCommits()
	for key, meta := range r.metadata {
		if meta.staged {
			meta.staged = false
			if success > r.priorCutSuccess {
				meta.committed = true
				meta.lastWrite = success
			}
		}
		if meta.refs == 0 && (!meta.committed || success-meta.lastWrite >= r.horizon) {
			delete(r.metadata, key)
		}
	}
	r.priorCutSuccess = success
}
