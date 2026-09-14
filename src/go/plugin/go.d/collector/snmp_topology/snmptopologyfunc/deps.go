// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

// Deps defines the dependency surface required by SNMP topology function handlers.
type Deps interface {
	Snapshot(topologyoptions.QueryOptions) (topologyv1.Data, bool, error)
	ManagedDeviceFocusTargets() []topologyoptions.ManagedFocusTarget
}
