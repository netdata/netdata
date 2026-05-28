// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

type buildSeqTransition uint8

const (
	buildSeqTransitionNone buildSeqTransition = iota
	buildSeqTransitionBroken
	buildSeqTransitionRecovered
)

type buildSeqObservation struct {
	transition buildSeqTransition
	previous   uint64
}

// observeBuildSuccessSeq tracks LastSuccessSeq monotonicity and reports transition events.
// BuildPlan uses this to avoid per-cycle warning spam on persistent sequence regressions.
func (e *Engine) observeBuildSuccessSeq(seq uint64) buildSeqObservation {
	state := &e.state.buildSeq
	obs := buildSeqObservation{previous: state.lastSuccess}
	if !state.initialized {
		state.initialized = true
		state.lastSuccess = seq
		state.violating = false
		return obs
	}

	if e.state.cfg.runtimePlanner {
		if seq < state.lastSuccess {
			if !state.violating {
				state.violating = true
				obs.transition = buildSeqTransitionBroken
			}
			return obs
		}
		if state.violating {
			state.violating = false
			obs.transition = buildSeqTransitionRecovered
		}
		if seq > state.lastSuccess {
			state.lastSuccess = seq
		}
		return obs
	}

	if seq <= state.lastSuccess {
		if !state.violating {
			state.violating = true
			obs.transition = buildSeqTransitionBroken
		}
		return obs
	}

	state.lastSuccess = seq
	if state.violating {
		state.violating = false
		obs.transition = buildSeqTransitionRecovered
	}
	return obs
}
