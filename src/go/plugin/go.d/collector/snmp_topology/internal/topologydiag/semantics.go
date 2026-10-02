// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

// Semantics shares production topology interpretation with offline diagnostics.
// Operations must not depend on live collector state.
type Semantics struct {
	// ReplayAcquisition borrows validated, immutable acquisition evidence.
	ReplayAcquisition func(*AcquisitionAttemptEvidence) (topologymodel.ObservationSnapshot, bool)
	// BuildGraph receives a newly assembled observation slice that it may sort.
	BuildGraph func([]topologymodel.ObservationSnapshot, string, topologyoptions.QueryOptions) (topologymodel.Data, bool, error)
}
