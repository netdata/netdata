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
	if err := opts.WorkLimiter.Charge(uint64(len(observations))); err != nil {
		return model.Result{}, err
	}
	state := newL2BuildState(len(observations), opts.WorkLimiter)
	if err := state.registerObservations(observations); err != nil {
		return model.Result{}, err
	}
	if opts.EnableLLDP {
		if err := state.applyLLDP(observations); err != nil {
			return model.Result{}, err
		}
	}
	if opts.EnableCDP {
		if err := state.applyCDP(observations); err != nil {
			return model.Result{}, err
		}
	}
	if opts.EnableSTP {
		if err := state.applySTP(observations); err != nil {
			return model.Result{}, err
		}
	}
	if opts.EnableBridge {
		if err := state.applyBridge(observations); err != nil {
			return model.Result{}, err
		}
	}
	if opts.EnableARP {
		if err := state.applyARP(observations); err != nil {
			return model.Result{}, err
		}
	}

	identityAliasStats, err := reconcileDeviceIdentityAliases(
		opts.WorkLimiter,
		state.devices,
		state.interfaces,
		state.enrichments,
		state.directOwnersByIP,
		state.remoteManagementByDeviceID,
		state.directManagementIPByDeviceID,
	)
	if err != nil {
		return model.Result{}, err
	}
	state.markManagedDevices()
	return state.buildResult(identityAliasStats, opts.CollectedAt)
}
