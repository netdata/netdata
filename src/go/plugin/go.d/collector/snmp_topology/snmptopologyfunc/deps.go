// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

// Deps defines the dependency surface required by SNMP topology function handlers.
type Deps interface {
	Snapshot(topologyoptions.QueryOptions) (SnapshotResult, bool, error)
	ManagedDeviceFocusTargets() []topologyoptions.ManagedFocusTarget
}

// SnapshotResult keeps graph data and selector choices from the same acquired generation.
type SnapshotResult struct {
	Data         topologyv1.Data
	FocusTargets []topologyoptions.ManagedFocusTarget
}
