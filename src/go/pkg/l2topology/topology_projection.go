// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import "github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"

// Projection is the L2 graph output consumed by topology producers.
type Projection = model.Projection

// OptionalValue keeps value presence explicit when a zero value is a real value.
type OptionalValue[T any] = model.OptionalValue[T]

// ProjectionActorDetail carries typed L2-owned actor facts keyed by graph actor id.
type ProjectionActorDetail = model.ProjectionActorDetail

type ProjectionDeviceActorDetail = model.ProjectionDeviceActorDetail

type ProjectionEndpointActorDetail = model.ProjectionEndpointActorDetail

type ProjectionSegmentActorDetail = model.ProjectionSegmentActorDetail

type ProjectionPortDetail = model.ProjectionPortDetail

type ProjectionPortNeighbor = model.ProjectionPortNeighbor

type ProjectionPortVLAN = model.ProjectionPortVLAN
