// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import "github.com/axiomhq/hyperloglog"

// interval contains no received member strings or individual observations.
type interval struct {
	count, sum, min, max float64
	quantiles            *percentiles
	members              *hyperloglog.Sketch
}

func (w *interval) reset() {
	w.count, w.sum, w.min, w.max = 0, 0, 0, 0
	if w.quantiles != nil {
		w.quantiles.reset()
	}
	if w.members != nil {
		w.members.Reset()
	}
}

// cardinality is the distinct-member estimate of a set interval; an interval
// without accepted members is exactly zero.
func (w *interval) cardinality() float64 {
	if w == nil || w.count == 0 {
		return 0
	}
	return float64(w.members.Estimate())
}

// prospectiveUpdate checks every basic result before admission or mutation.
func prospectiveUpdate(r record, e *series) (value, count, sum float64, err error) {
	switch r.kind {
	case counter:
		value = r.value / r.rate
		if e != nil {
			value += e.value
		}
	case gauge:
		value = r.value
		if r.delta {
			value += e.value
		} // baseline eligibility is checked by admission.
	case timer, histogram:
		count = 1 / r.rate
		sum = r.value * count
		// Check the contribution too: signed cancellation cannot recover overflow.
		if !finite(count) || !finite(sum) {
			return 0, 0, 0, rejectOverflow
		}
		if e != nil && e.window != nil {
			count += e.window.count
			sum += e.window.sum
		}
	}
	if !finite(value) || !finite(count) || !finite(sum) {
		return 0, 0, 0, rejectOverflow
	}
	return value, count, sum, nil
}

func (e *series) update(r record, value, count, sum float64) {
	if r.kind == counter || r.kind == gauge {
		e.value = value
		return
	}
	if e.window == nil {
		e.window = &interval{}
	}
	w := e.window
	if r.kind == set {
		if w.members == nil {
			// Fixed valid precision, shared dependency's bounded sparse/dense implementation.
			w.members, _ = hyperloglog.NewSketch(12, true)
		}
		w.members.Insert([]byte(r.member))
		w.count = 1 // Tracks nonempty input, not an exact membership count.
		return
	}
	if w.count == 0 {
		w.min, w.max = r.value, r.value
	} else {
		w.min = min(w.min, r.value)
		w.max = max(w.max, r.value)
	}
	w.count, w.sum = count, sum
	if w.quantiles == nil {
		w.quantiles = newPercentiles(percentileBins)
	}
	w.quantiles.add(r.value, r.rate)
}
