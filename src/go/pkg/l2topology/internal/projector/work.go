// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
)

func chargeProjectionPreparation(result model.Result, limiter worklimit.Limiter) error {
	if limiter == nil {
		return nil
	}
	items, err := projectionResultItems(result)
	if err != nil {
		return err
	}
	if err := limiter.Charge(items); err != nil {
		return err
	}
	var addresses uint64
	for _, device := range result.Devices {
		addresses, err = worklimit.Sum(addresses, uint64(len(device.Addresses)+1))
		if err != nil {
			return err
		}
	}
	if err := limiter.ChargeSort(addresses); err != nil {
		return err
	}
	// Each interface normalizes at most four name aliases before indexing.
	return limiter.ChargeProduct(uint64(len(result.Interfaces)), 8)
}

func chargeBridgeCollection(result model.Result, config topologyInferenceStrategyConfig, limiter worklimit.Limiter) error {
	if limiter == nil {
		return nil
	}
	items, err := worklimit.Sum(
		uint64(len(result.Devices)), uint64(len(result.Interfaces)),
		uint64(len(result.Adjacencies)), uint64(len(result.Attachments)),
	)
	if err != nil {
		return err
	}
	if err := limiter.Charge(items); err != nil {
		return err
	}
	if !config.enableFDBPairwiseLinks {
		return limiter.ChargeSort(uint64(len(result.Attachments)))
	}
	// Pairwise FDB inference can compare every reporter attachment with every
	// managed reporter before it collapses the compatible pair set.
	fanout, err := worklimit.Product(uint64(len(result.Attachments)), uint64(len(result.Devices)))
	if err != nil {
		return err
	}
	if err := limiter.Charge(fanout); err != nil {
		return err
	}
	if err := limiter.ChargeSort(fanout); err != nil {
		return err
	}
	return limiter.ChargeSort(uint64(len(result.Attachments)))
}

func chargeLinearProjectionStage(limiter worklimit.Limiter, sizes ...int) error {
	if limiter == nil {
		return nil
	}
	var items uint64
	for _, size := range sizes {
		var err error
		items, err = worklimit.Sum(items, uint64(size))
		if err != nil {
			return err
		}
	}
	if err := limiter.Charge(items); err != nil {
		return err
	}
	return limiter.ChargeSort(items)
}

func chargeSegmentProjection(builder *graphBuilder) error {
	if builder == nil || builder.opts.WorkLimiter == nil {
		return nil
	}
	items, err := worklimit.Sum(
		uint64(len(builder.result.Attachments)), uint64(len(builder.result.Adjacencies)),
		uint64(len(builder.bridgeLinks)), uint64(len(builder.endpointActors.actors)),
	)
	if err != nil {
		return err
	}
	// Current bridge-domain merging and endpoint-to-segment assignment have a
	// quadratic worst-case envelope in their actual candidate vector.
	if err := builder.opts.WorkLimiter.ChargeProduct(items, items); err != nil {
		return err
	}
	return builder.opts.WorkLimiter.ChargeSort(items)
}

func chargeProjectionFinalization(builder *graphBuilder) error {
	if builder == nil || builder.opts.WorkLimiter == nil {
		return nil
	}
	actors, err := worklimit.Sum(
		uint64(len(builder.actors)), uint64(len(builder.segmentProjection.actors)),
	)
	if err != nil {
		return err
	}
	links, err := worklimit.Sum(
		uint64(len(builder.projectedAdjacencies.links)), uint64(len(builder.segmentProjection.links)),
	)
	if err != nil {
		return err
	}
	items, err := worklimit.Sum(actors, links)
	if err != nil {
		return err
	}
	// Finalization makes several complete pruning, identity, display, and stats
	// passes, plus two deterministic actor/link sorts.
	if err := builder.opts.WorkLimiter.ChargeProduct(items, 8); err != nil {
		return err
	}
	if err := builder.opts.WorkLimiter.ChargeSort(actors); err != nil {
		return err
	}
	if err := builder.opts.WorkLimiter.ChargeSort(actors); err != nil {
		return err
	}
	if err := builder.opts.WorkLimiter.ChargeSort(links); err != nil {
		return err
	}
	return builder.opts.WorkLimiter.ChargeSort(links)
}

func projectionResultItems(result model.Result) (uint64, error) {
	return worklimit.Sum(
		uint64(len(result.Devices)), uint64(len(result.Interfaces)), uint64(len(result.Adjacencies)),
		uint64(len(result.Attachments)), uint64(len(result.Enrichments)),
	)
}
