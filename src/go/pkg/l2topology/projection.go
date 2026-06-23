// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/projector"
)

// GraphOptions controls conversion from Result to the internal graph projection.
type GraphOptions = model.GraphOptions

// Projection is the L2 graph output consumed by topology producers.
type Projection = model.Projection

// ProjectionStats summarizes the graph projection produced from an L2 result.
type ProjectionStats = model.ProjectionStats

// OptionalValue keeps value presence explicit when a zero value is a real value.
type OptionalValue[T any] = model.OptionalValue[T]

// ProjectionActorDetail carries typed L2-owned actor facts keyed by graph actor id.
type ProjectionActorDetail = model.ProjectionActorDetail

// ProjectionDeviceActorDetail carries typed facts for managed device actors.
type ProjectionDeviceActorDetail = model.ProjectionDeviceActorDetail

// ProjectionEndpointActorDetail carries typed facts for inferred endpoint actors.
type ProjectionEndpointActorDetail = model.ProjectionEndpointActorDetail

// ProjectionSegmentActorDetail carries typed facts for inferred segment actors.
type ProjectionSegmentActorDetail = model.ProjectionSegmentActorDetail

// ProjectionPortDetail carries typed per-port inventory for managed device actors.
type ProjectionPortDetail = model.ProjectionPortDetail

// ProjectionPortNeighbor carries typed per-port neighbor summary facts.
type ProjectionPortNeighbor = model.ProjectionPortNeighbor

// ProjectionPortVLAN carries typed per-port VLAN facts.
type ProjectionPortVLAN = model.ProjectionPortVLAN

// ToGraph converts an L2 topology result to the graph projection consumed by
// topology producers.
func ToGraph(result Result, opts GraphOptions) Projection {
	return projector.ToGraph(result, opts)
}

// IsDeviceActorType reports whether actorType is one of the managed device
// actor types emitted by the L2 graph projection.
func IsDeviceActorType(actorType string) bool {
	return projector.IsDeviceActorType(actorType)
}
