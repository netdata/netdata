// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/stretchr/testify/require"
)

func TestChargeBridgeCollectionChargesPairwiseRecordSortBeforeInference(t *testing.T) {
	const size = 8
	result := model.Result{
		Devices:     make([]model.Device, size),
		Attachments: make([]model.Attachment, size),
	}
	// Existing linear preparation + fanout + attachment sorting fits this
	// budget. Sorting the product-sized candidate vector does not.
	const budget = uint64(104)
	remaining := budget
	limitErr := errors.New("pairwise work exhausted")
	err := chargeBridgeCollection(result, topologyInferenceStrategyConfig{enableFDBPairwiseLinks: true}, func(units uint64) error {
		if units > remaining {
			return limitErr
		}
		remaining -= units
		return nil
	})
	require.ErrorIs(t, err, limitErr)
}
