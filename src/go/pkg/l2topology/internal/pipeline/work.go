// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
)

func chargePipelineInput(observations []model.L2Observation, opts model.DiscoverOptions) error {
	limiter := opts.WorkLimiter
	if limiter == nil {
		return nil
	}
	if err := limiter.Charge(uint64(len(observations))); err != nil {
		return err
	}
	for _, observation := range observations {
		if err := chargeLinearWork(limiter,
			len(observation.ManagementAliases),
			len(observation.Labels),
			len(observation.Interfaces),
		); err != nil {
			return err
		}
		if opts.EnableLLDP {
			if err := chargeSortedInput(limiter, len(observation.LLDPRemotes), 1); err != nil {
				return err
			}
		}
		if opts.EnableCDP {
			if err := chargeSortedInput(limiter, len(observation.CDPRemotes), 1); err != nil {
				return err
			}
		}
		if opts.EnableSTP {
			if err := chargeSortedInput(limiter, len(observation.BridgePorts), 1); err != nil {
				return err
			}
			if err := chargeSortedInput(limiter, len(observation.STPPorts), 1); err != nil {
				return err
			}
		}
		if opts.EnableBridge {
			if err := chargeSortedInput(limiter, len(observation.BridgePorts), 1); err != nil {
				return err
			}
			// FDB normalization sorts the filtered input and the deduplicated
			// candidate vector; both are bounded by the captured row count.
			if err := chargeSortedInput(limiter, len(observation.FDBEntries), 2); err != nil {
				return err
			}
		}
		if opts.EnableARP {
			if err := chargeSortedInput(limiter, len(observation.ARPNDEntries), 1); err != nil {
				return err
			}
		}
	}
	return nil
}

func chargeIdentityReconciliation(limiter worklimit.Limiter, state *l2BuildState) error {
	if limiter == nil || state == nil {
		return nil
	}
	items, err := worklimit.Sum(
		uint64(len(state.devices)), uint64(len(state.interfaces)), uint64(len(state.enrichments)),
		uint64(len(state.remoteManagementByDeviceID)), uint64(len(state.directOwnersByIP)),
	)
	if err != nil {
		return err
	}
	// Alias reconciliation includes data-dependent ownership joins and map
	// ordering. Charge its current quadratic envelope before it executes.
	return limiter.ChargeProduct(items, items)
}

func chargePipelineResult(limiter worklimit.Limiter, state *l2BuildState) error {
	if limiter == nil || state == nil {
		return nil
	}
	for _, size := range [...]int{
		len(state.devices), len(state.interfaces), len(state.adjacencies),
		len(state.attachments), len(state.enrichments),
	} {
		if err := chargeSortedInput(limiter, size, 1); err != nil {
			return err
		}
	}
	if err := limiter.Charge(uint64(len(state.enrichments))); err != nil {
		return err
	}
	for _, enrichment := range state.enrichments {
		if enrichment == nil {
			continue
		}
		for _, size := range [...]int{
			len(enrichment.IPs), len(enrichment.Protocols), len(enrichment.DeviceIDs),
			len(enrichment.IfIndexes), len(enrichment.IfNames), len(enrichment.States), len(enrichment.AddrTypes),
		} {
			if err := limiter.ChargeSort(uint64(size)); err != nil {
				return err
			}
		}
	}
	return nil
}

func chargeSortedInput(limiter worklimit.Limiter, size, executions int) error {
	for range executions {
		if err := limiter.Charge(uint64(size)); err != nil {
			return err
		}
		if err := limiter.ChargeSort(uint64(size)); err != nil {
			return err
		}
	}
	return nil
}

func chargeLinearWork(limiter worklimit.Limiter, sizes ...int) error {
	for _, size := range sizes {
		if err := limiter.Charge(uint64(size)); err != nil {
			return err
		}
	}
	return nil
}
