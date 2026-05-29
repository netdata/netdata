// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import "github.com/netdata/netdata/go/plugins/pkg/topology"

// Deps defines the dependency surface required by Cato Networks function handlers.
type Deps interface {
	CurrentTopology() (*topology.Data, bool)
}
