// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"

// Deps defines the dependency surface required by Cato Networks function handlers.
type Deps interface {
	CurrentTopology() (*topologyv1.Data, bool)
}
