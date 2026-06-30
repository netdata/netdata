// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

const (
	fdbStatusLearned = "learned"
	fdbStatusSelf    = "self"
	fdbStatusIgnored = "ignored"
)

// BuildL2ResultFromObservations converts normalized L2 observations into a
// deterministic L2 topology result. Callers that need a stable timestamp should
// set model.DiscoverOptions.CollectedAt explicitly.
func BuildL2ResultFromObservations(observations []model.L2Observation, opts model.DiscoverOptions) (model.Result, error) {
	if len(observations) == 0 {
		return model.Result{}, errors.New("at least one observation is required")
	}
	if !opts.EnableLLDP && !opts.EnableCDP && !opts.EnableBridge && !opts.EnableARP && !opts.EnableSTP {
		opts.EnableLLDP = true
		opts.EnableCDP = true
	}

	state := newL2BuildState(len(observations))
	if err := state.registerObservations(observations); err != nil {
		return model.Result{}, err
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
