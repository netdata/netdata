// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "fmt"

const (
	fdbStatusLearned = "learned"
	fdbStatusSelf    = "self"
	fdbStatusIgnored = "ignored"
)

// BuildL2ResultFromObservations converts normalized L2 observations into a
// deterministic engine result. Callers that need a stable timestamp should set
// DiscoverOptions.CollectedAt explicitly.
func BuildL2ResultFromObservations(observations []L2Observation, opts DiscoverOptions) (Result, error) {
	if len(observations) == 0 {
		return Result{}, fmt.Errorf("%w: at least one observation is required", ErrInvalidRequest)
	}
	if !opts.EnableLLDP && !opts.EnableCDP && !opts.EnableBridge && !opts.EnableARP && !opts.EnableSTP {
		opts.EnableLLDP = true
		opts.EnableCDP = true
	}

	state := newL2BuildState(len(observations))
	if err := state.registerObservations(observations); err != nil {
		return Result{}, err
	}
	if opts.EnableLLDP {
		state.applyLLDP(observations)
	}
	if opts.EnableCDP {
		state.applyCDP(observations)
	}
	if opts.EnableSTP {
		state.applySTP(observations)
	}
	if opts.EnableBridge {
		state.applyBridge(observations)
	}
	if opts.EnableARP {
		state.applyARP(observations)
	}

	identityAliasStats := reconcileDeviceIdentityAliases(state.devices, state.interfaces, state.enrichments)
	state.markManagedDevices()

	return state.buildResult(identityAliasStats, opts.CollectedAt), nil
}
