// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/projector"
)

// GraphOptions controls conversion from Result to the internal graph projection.
type GraphOptions = model.GraphOptions

// ToGraph converts an L2 topology result to the graph projection consumed by
// topology producers.
func ToGraph(result Result, opts GraphOptions) Projection {
	return projector.ToGraph(result, opts)
}

func IsDeviceActorType(actorType string) bool {
	return projector.IsDeviceActorType(actorType)
}
