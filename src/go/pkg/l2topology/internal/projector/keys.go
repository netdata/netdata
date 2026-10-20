// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/keyutil"
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

const keySep = keyutil.Sep

const (
	adjacencyLabelPairID   = model.AdjacencyLabelPairID
	adjacencyLabelPairPass = model.AdjacencyLabelPairPass
)

func deviceIfIndexKey(deviceID string, ifIndex int) string {
	return keyutil.DeviceIfIndexKey(deviceID, ifIndex)
}
